package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct{ conn *sql.DB }

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	conn, err := sql.Open("sqlite", filepath.Join(dataDir, "telegraph.db"))
	if err != nil {
		return nil, err
	}
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.SetMaxOpenConns(4)
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) Conn() *sql.DB { return db.conn }
func (db *DB) Close() error  { return db.conn.Close() }

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id TEXT PRIMARY KEY,
    event_name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret TEXT DEFAULT '',
    active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_subs_event ON subscriptions(event_name);

CREATE TABLE IF NOT EXISTS deliveries (
    id TEXT PRIMARY KEY,
    subscription_id TEXT NOT NULL,
    event_name TEXT NOT NULL,
    payload TEXT DEFAULT '{}',
    status TEXT DEFAULT 'pending',
    status_code INTEGER DEFAULT 0,
    response_ms INTEGER DEFAULT 0,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    last_error TEXT DEFAULT '',
    next_retry_at TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now')),
    completed_at TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_del_sub ON deliveries(subscription_id);
CREATE INDEX IF NOT EXISTS idx_del_event ON deliveries(event_name);
CREATE INDEX IF NOT EXISTS idx_del_status ON deliveries(status);
CREATE INDEX IF NOT EXISTS idx_del_retry ON deliveries(status, next_retry_at);
`)
	return err
}

// --- Event types ---

type Event struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	SubCount    int    `json:"subscriber_count"`
}

func (db *DB) CreateEvent(name, desc string) (*Event, error) {
	id := "evt_" + genID(6)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.conn.Exec("INSERT INTO events (id,name,description,created_at) VALUES (?,?,?,?)", id, name, desc, now)
	if err != nil {
		return nil, err
	}
	return &Event{ID: id, Name: name, Description: desc, CreatedAt: now}, nil
}

func (db *DB) ListEvents() ([]Event, error) {
	rows, err := db.conn.Query(`SELECT e.id, e.name, e.description, e.created_at,
		(SELECT COUNT(*) FROM subscriptions WHERE event_name=e.name AND active=1)
		FROM events e ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		rows.Scan(&e.ID, &e.Name, &e.Description, &e.CreatedAt, &e.SubCount)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (db *DB) GetEvent(name string) (*Event, error) {
	var e Event
	err := db.conn.QueryRow(`SELECT e.id, e.name, e.description, e.created_at,
		(SELECT COUNT(*) FROM subscriptions WHERE event_name=e.name AND active=1)
		FROM events e WHERE e.name=?`, name).
		Scan(&e.ID, &e.Name, &e.Description, &e.CreatedAt, &e.SubCount)
	return &e, err
}

func (db *DB) DeleteEvent(name string) error {
	db.conn.Exec("DELETE FROM subscriptions WHERE event_name=?", name)
	db.conn.Exec("DELETE FROM deliveries WHERE event_name=?", name)
	_, err := db.conn.Exec("DELETE FROM events WHERE name=?", name)
	return err
}

// --- Subscriptions ---

type Subscription struct {
	ID        string `json:"id"`
	EventName string `json:"event_name"`
	URL       string `json:"url"`
	Secret    string `json:"secret,omitempty"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func (db *DB) CreateSubscription(eventName, url, secret string) (*Subscription, error) {
	id := "sub_" + genID(8)
	now := time.Now().UTC().Format(time.RFC3339)
	if secret == "" {
		secret = genID(16)
	}
	_, err := db.conn.Exec("INSERT INTO subscriptions (id,event_name,url,secret,created_at) VALUES (?,?,?,?,?)",
		id, eventName, url, secret, now)
	if err != nil {
		return nil, err
	}
	return &Subscription{ID: id, EventName: eventName, URL: url, Secret: secret, Active: true, CreatedAt: now}, nil
}

func (db *DB) ListSubscriptions(eventName string) ([]Subscription, error) {
	rows, err := db.conn.Query("SELECT id,event_name,url,secret,active,created_at FROM subscriptions WHERE event_name=? AND active=1 ORDER BY created_at", eventName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var s Subscription
		var act int
		rows.Scan(&s.ID, &s.EventName, &s.URL, &s.Secret, &act, &s.CreatedAt)
		s.Active = act == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) GetSubscription(id string) (*Subscription, error) {
	var s Subscription
	var act int
	err := db.conn.QueryRow("SELECT id,event_name,url,secret,active,created_at FROM subscriptions WHERE id=?", id).
		Scan(&s.ID, &s.EventName, &s.URL, &s.Secret, &act, &s.CreatedAt)
	s.Active = act == 1
	return &s, err
}

func (db *DB) DeleteSubscription(id string) error {
	_, err := db.conn.Exec("DELETE FROM subscriptions WHERE id=?", id)
	return err
}

func (db *DB) TotalSubscriptions() int {
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE active=1").Scan(&count)
	return count
}

// --- Deliveries ---

type Delivery struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscription_id"`
	EventName      string `json:"event_name"`
	Payload        string `json:"payload"`
	Status         string `json:"status"`
	StatusCode     int    `json:"status_code"`
	ResponseMs     int    `json:"response_ms"`
	Attempts       int    `json:"attempts"`
	MaxAttempts    int    `json:"max_attempts"`
	LastError      string `json:"last_error,omitempty"`
	NextRetryAt    string `json:"next_retry_at,omitempty"`
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

func (db *DB) CreateDelivery(subID, eventName, payload string, maxAttempts int) (*Delivery, error) {
	id := "dlv_" + genID(10)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.conn.Exec(`INSERT INTO deliveries (id,subscription_id,event_name,payload,max_attempts,created_at)
		VALUES (?,?,?,?,?,?)`, id, subID, eventName, payload, maxAttempts, now)
	if err != nil {
		return nil, err
	}
	return &Delivery{ID: id, SubscriptionID: subID, EventName: eventName, Payload: payload,
		Status: "pending", MaxAttempts: maxAttempts, CreatedAt: now}, nil
}

func (db *DB) ListDeliveries(limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return db.queryDeliveries("SELECT id,subscription_id,event_name,payload,status,status_code,response_ms,attempts,max_attempts,last_error,next_retry_at,created_at,completed_at FROM deliveries ORDER BY created_at DESC LIMIT ?", limit)
}

func (db *DB) ListDeliveriesByEvent(eventName string, limit int) ([]Delivery, error) {
	if limit <= 0 {
		limit = 100
	}
	return db.queryDeliveries("SELECT id,subscription_id,event_name,payload,status,status_code,response_ms,attempts,max_attempts,last_error,next_retry_at,created_at,completed_at FROM deliveries WHERE event_name=? ORDER BY created_at DESC LIMIT ?", eventName, limit)
}

func (db *DB) GetDelivery(id string) (*Delivery, error) {
	rows, err := db.queryDeliveries("SELECT id,subscription_id,event_name,payload,status,status_code,response_ms,attempts,max_attempts,last_error,next_retry_at,created_at,completed_at FROM deliveries WHERE id=?", id)
	if err != nil || len(rows) == 0 {
		if err == nil {
			err = fmt.Errorf("not found")
		}
		return nil, err
	}
	return &rows[0], nil
}

func (db *DB) UpdateDeliverySuccess(id string, statusCode, responseMs int) {
	now := time.Now().UTC().Format(time.RFC3339)
	db.conn.Exec("UPDATE deliveries SET status='delivered', status_code=?, response_ms=?, attempts=attempts+1, completed_at=? WHERE id=?",
		statusCode, responseMs, now, id)
}

func (db *DB) UpdateDeliveryFail(id string, statusCode, responseMs int, errMsg string, maxAttempts int) {
	var attempts int
	db.conn.QueryRow("SELECT attempts FROM deliveries WHERE id=?", id).Scan(&attempts)
	attempts++

	if attempts >= maxAttempts {
		now := time.Now().UTC().Format(time.RFC3339)
		db.conn.Exec("UPDATE deliveries SET status='failed', status_code=?, response_ms=?, attempts=?, last_error=?, completed_at=? WHERE id=?",
			statusCode, responseMs, attempts, errMsg, now, id)
	} else {
		nextRetry := time.Now().Add(time.Duration(60*attempts) * time.Second).UTC().Format(time.RFC3339)
		db.conn.Exec("UPDATE deliveries SET status='retrying', status_code=?, response_ms=?, attempts=?, last_error=?, next_retry_at=? WHERE id=?",
			statusCode, responseMs, attempts, errMsg, nextRetry, id)
	}
}

func (db *DB) ClaimRetryDeliveries() ([]Delivery, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	return db.queryDeliveries(`SELECT id,subscription_id,event_name,payload,status,status_code,response_ms,attempts,max_attempts,last_error,next_retry_at,created_at,completed_at
		FROM deliveries WHERE status='retrying' AND next_retry_at<=? ORDER BY next_retry_at LIMIT 10`, now)
}

func (db *DB) RetryDelivery(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.conn.Exec("UPDATE deliveries SET status='retrying', next_retry_at=?, attempts=0, last_error='' WHERE id=? AND status='failed'", now, id)
	return err
}

func (db *DB) queryDeliveries(query string, args ...any) ([]Delivery, error) {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		rows.Scan(&d.ID, &d.SubscriptionID, &d.EventName, &d.Payload, &d.Status, &d.StatusCode,
			&d.ResponseMs, &d.Attempts, &d.MaxAttempts, &d.LastError, &d.NextRetryAt,
			&d.CreatedAt, &d.CompletedAt)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (db *DB) MonthlyFireCount() (int, error) {
	cutoff := time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")
	var count int
	err := db.conn.QueryRow("SELECT COUNT(DISTINCT id) FROM deliveries WHERE created_at>=?", cutoff).Scan(&count)
	return count, err
}

// --- Stats ---

func (db *DB) Stats() map[string]any {
	var events, subs, deliveries, delivered, failed int
	db.conn.QueryRow("SELECT COUNT(*) FROM events").Scan(&events)
	db.conn.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE active=1").Scan(&subs)
	db.conn.QueryRow("SELECT COUNT(*) FROM deliveries").Scan(&deliveries)
	db.conn.QueryRow("SELECT COUNT(*) FROM deliveries WHERE status='delivered'").Scan(&delivered)
	db.conn.QueryRow("SELECT COUNT(*) FROM deliveries WHERE status='failed'").Scan(&failed)
	return map[string]any{"events": events, "subscriptions": subs, "deliveries": deliveries, "delivered": delivered, "failed": failed}
}

func (db *DB) Cleanup(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	res, err := db.conn.Exec("DELETE FROM deliveries WHERE created_at < ? AND status IN ('delivered','failed')", cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func genID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
