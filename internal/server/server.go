package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/stockyard-dev/stockyard-telegraph/internal/store"
)

type Server struct {
	db     *store.DB
	mux    *http.ServeMux
	port   int
	limits Limits
	client *http.Client
	stop   chan struct{}
}

func New(db *store.DB, port int, limits Limits) *Server {
	s := &Server{
		db:     db,
		mux:    http.NewServeMux(),
		port:   port,
		limits: limits,
		client: &http.Client{Timeout: 15 * time.Second},
		stop:   make(chan struct{}),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Events
	s.mux.HandleFunc("POST /api/events", s.handleCreateEvent)
	s.mux.HandleFunc("GET /api/events", s.handleListEvents)
	s.mux.HandleFunc("GET /api/events/{name}", s.handleGetEvent)
	s.mux.HandleFunc("DELETE /api/events/{name}", s.handleDeleteEvent)

	// Subscriptions
	s.mux.HandleFunc("POST /api/events/{name}/subscriptions", s.handleCreateSub)
	s.mux.HandleFunc("GET /api/events/{name}/subscriptions", s.handleListSubs)
	s.mux.HandleFunc("DELETE /api/subscriptions/{id}", s.handleDeleteSub)

	// Fire
	s.mux.HandleFunc("POST /api/events/{name}/fire", s.handleFire)

	// Deliveries
	s.mux.HandleFunc("GET /api/deliveries", s.handleListDeliveries)
	s.mux.HandleFunc("GET /api/events/{name}/deliveries", s.handleListEventDeliveries)
	s.mux.HandleFunc("GET /api/deliveries/{id}", s.handleGetDelivery)
	s.mux.HandleFunc("POST /api/deliveries/{id}/retry", s.handleRetryDelivery)

	// Status
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ui", s.handleUI)

	s.mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"product": "stockyard-telegraph", "version": "0.1.0"})
	})
}

func (s *Server) Start() error {
	if s.limits.RetryDeliveries {
		go s.retryLoop()
		log.Printf("[telegraph] retry worker started")
	}
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[telegraph] listening on %s", addr)
	return http.ListenAndServe(addr, s.mux)
}

// --- Retry worker ---

func (s *Server) retryLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.processRetries()
		}
	}
}

func (s *Server) processRetries() {
	deliveries, err := s.db.ClaimRetryDeliveries()
	if err != nil || len(deliveries) == 0 {
		return
	}
	for _, d := range deliveries {
		sub, err := s.db.GetSubscription(d.SubscriptionID)
		if err != nil {
			continue
		}
		go s.deliver(&d, sub)
	}
}

// --- Fire event ---

func (s *Server) handleFire(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.db.GetEvent(name); err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}

	// Read payload
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if len(body) == 0 {
		body = []byte("{}")
	}

	subs, err := s.db.ListSubscriptions(name)
	if err != nil || len(subs) == 0 {
		writeJSON(w, 200, map[string]any{"fired": 0, "message": "no subscribers"})
		return
	}

	maxAttempts := 1
	if s.limits.RetryDeliveries {
		maxAttempts = 3
	}

	var deliveryIDs []string
	for _, sub := range subs {
		d, err := s.db.CreateDelivery(sub.ID, name, string(body), maxAttempts)
		if err != nil {
			continue
		}
		deliveryIDs = append(deliveryIDs, d.ID)
		go s.deliver(d, &sub)
	}

	writeJSON(w, 200, map[string]any{"fired": len(deliveryIDs), "delivery_ids": deliveryIDs})
}

func (s *Server) deliver(d *store.Delivery, sub *store.Subscription) {
	payload := []byte(d.Payload)

	req, err := http.NewRequest("POST", sub.URL, bytes.NewReader(payload))
	if err != nil {
		s.db.UpdateDeliveryFail(d.ID, 0, 0, err.Error(), d.MaxAttempts)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegraph-Event", d.EventName)
	req.Header.Set("X-Telegraph-Delivery", d.ID)
	req.Header.Set("X-Telegraph-Timestamp", time.Now().UTC().Format(time.RFC3339))

	// HMAC signing (Pro)
	if s.limits.HMACSigning && sub.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sub.Secret))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Telegraph-Signature", "sha256="+sig)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	responseMs := int(time.Since(start).Milliseconds())

	if err != nil {
		s.db.UpdateDeliveryFail(d.ID, 0, responseMs, err.Error(), d.MaxAttempts)
		log.Printf("[deliver] %s → %s FAIL: %v", d.ID, sub.URL, err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.db.UpdateDeliverySuccess(d.ID, resp.StatusCode, responseMs)
		log.Printf("[deliver] %s → %s OK (%d, %dms)", d.ID, sub.URL, resp.StatusCode, responseMs)
	} else {
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		s.db.UpdateDeliveryFail(d.ID, resp.StatusCode, responseMs, errMsg, d.MaxAttempts)
		log.Printf("[deliver] %s → %s FAIL: %s (%dms)", d.ID, sub.URL, errMsg, responseMs)
	}
}

// --- Event handlers ---

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}

	if s.limits.MaxEvents > 0 {
		events, _ := s.db.ListEvents()
		if LimitReached(s.limits.MaxEvents, len(events)) {
			writeJSON(w, 402, map[string]string{
				"error":   fmt.Sprintf("free tier limit: %d events max — upgrade to Pro", s.limits.MaxEvents),
				"upgrade": "https://stockyard.dev/telegraph/",
			})
			return
		}
	}

	evt, err := s.db.CreateEvent(req.Name, req.Description)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"event": evt})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.db.ListEvents()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if events == nil {
		events = []store.Event{}
	}
	writeJSON(w, 200, map[string]any{"events": events, "count": len(events)})
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	evt, err := s.db.GetEvent(r.PathValue("name"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"event": evt})
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.db.GetEvent(name); err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}
	s.db.DeleteEvent(name)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// --- Subscription handlers ---

func (s *Server) handleCreateSub(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.db.GetEvent(name); err != nil {
		writeJSON(w, 404, map[string]string{"error": "event not found"})
		return
	}

	var req struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.URL == "" {
		writeJSON(w, 400, map[string]string{"error": "url is required"})
		return
	}

	if s.limits.MaxSubscriptions > 0 {
		total := s.db.TotalSubscriptions()
		if LimitReached(s.limits.MaxSubscriptions, total) {
			writeJSON(w, 402, map[string]string{
				"error":   fmt.Sprintf("free tier limit: %d subscriptions max — upgrade to Pro", s.limits.MaxSubscriptions),
				"upgrade": "https://stockyard.dev/telegraph/",
			})
			return
		}
	}

	sub, err := s.db.CreateSubscription(name, req.URL, req.Secret)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"subscription": sub})
}

func (s *Server) handleListSubs(w http.ResponseWriter, r *http.Request) {
	subs, err := s.db.ListSubscriptions(r.PathValue("name"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if subs == nil {
		subs = []store.Subscription{}
	}
	writeJSON(w, 200, map[string]any{"subscriptions": subs, "count": len(subs)})
}

func (s *Server) handleDeleteSub(w http.ResponseWriter, r *http.Request) {
	s.db.DeleteSubscription(r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// --- Delivery handlers ---

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	dels, err := s.db.ListDeliveries(limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if dels == nil {
		dels = []store.Delivery{}
	}
	writeJSON(w, 200, map[string]any{"deliveries": dels, "count": len(dels)})
}

func (s *Server) handleListEventDeliveries(w http.ResponseWriter, r *http.Request) {
	dels, err := s.db.ListDeliveriesByEvent(r.PathValue("name"), 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if dels == nil {
		dels = []store.Delivery{}
	}
	writeJSON(w, 200, map[string]any{"deliveries": dels, "count": len(dels)})
}

func (s *Server) handleGetDelivery(w http.ResponseWriter, r *http.Request) {
	d, err := s.db.GetDelivery(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "delivery not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"delivery": d})
}

func (s *Server) handleRetryDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.RetryDelivery(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "retrying"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.db.Stats())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
