package store
import ("database/sql";"fmt";"os";"path/filepath";"time";_ "modernc.org/sqlite")
type DB struct{db *sql.DB}
type Webhook struct {
	ID string `json:"id"`
	Name string `json:"name"`
	SourceURL string `json:"source_url"`
	TargetURL string `json:"target_url"`
	Events string `json:"events"`
	Status string `json:"status"`
	DeliveryCount int `json:"delivery_count"`
	LastDeliveryAt string `json:"last_delivery_at"`
	CreatedAt string `json:"created_at"`
}
func Open(d string)(*DB,error){if err:=os.MkdirAll(d,0755);err!=nil{return nil,err};db,err:=sql.Open("sqlite",filepath.Join(d,"telegraph.db")+"?_journal_mode=WAL&_busy_timeout=5000");if err!=nil{return nil,err}
db.Exec(`CREATE TABLE IF NOT EXISTS webhooks(id TEXT PRIMARY KEY,name TEXT NOT NULL,source_url TEXT DEFAULT '',target_url TEXT DEFAULT '',events TEXT DEFAULT '[]',status TEXT DEFAULT 'active',delivery_count INTEGER DEFAULT 0,last_delivery_at TEXT DEFAULT '',created_at TEXT DEFAULT(datetime('now')))`)
return &DB{db:db},nil}
func(d *DB)Close()error{return d.db.Close()}
func genID()string{return fmt.Sprintf("%d",time.Now().UnixNano())}
func now()string{return time.Now().UTC().Format(time.RFC3339)}
func(d *DB)Create(e *Webhook)error{e.ID=genID();e.CreatedAt=now();_,err:=d.db.Exec(`INSERT INTO webhooks(id,name,source_url,target_url,events,status,delivery_count,last_delivery_at,created_at)VALUES(?,?,?,?,?,?,?,?,?)`,e.ID,e.Name,e.SourceURL,e.TargetURL,e.Events,e.Status,e.DeliveryCount,e.LastDeliveryAt,e.CreatedAt);return err}
func(d *DB)Get(id string)*Webhook{var e Webhook;if d.db.QueryRow(`SELECT id,name,source_url,target_url,events,status,delivery_count,last_delivery_at,created_at FROM webhooks WHERE id=?`,id).Scan(&e.ID,&e.Name,&e.SourceURL,&e.TargetURL,&e.Events,&e.Status,&e.DeliveryCount,&e.LastDeliveryAt,&e.CreatedAt)!=nil{return nil};return &e}
func(d *DB)List()[]Webhook{rows,_:=d.db.Query(`SELECT id,name,source_url,target_url,events,status,delivery_count,last_delivery_at,created_at FROM webhooks ORDER BY created_at DESC`);if rows==nil{return nil};defer rows.Close();var o []Webhook;for rows.Next(){var e Webhook;rows.Scan(&e.ID,&e.Name,&e.SourceURL,&e.TargetURL,&e.Events,&e.Status,&e.DeliveryCount,&e.LastDeliveryAt,&e.CreatedAt);o=append(o,e)};return o}
func(d *DB)Update(e *Webhook)error{_,err:=d.db.Exec(`UPDATE webhooks SET name=?,source_url=?,target_url=?,events=?,status=?,delivery_count=?,last_delivery_at=? WHERE id=?`,e.Name,e.SourceURL,e.TargetURL,e.Events,e.Status,e.DeliveryCount,e.LastDeliveryAt,e.ID);return err}
func(d *DB)Delete(id string)error{_,err:=d.db.Exec(`DELETE FROM webhooks WHERE id=?`,id);return err}
func(d *DB)Count()int{var n int;d.db.QueryRow(`SELECT COUNT(*) FROM webhooks`).Scan(&n);return n}

func(d *DB)Search(q string, filters map[string]string)[]Webhook{
    where:="1=1"
    args:=[]any{}
    if q!=""{
        where+=" AND (name LIKE ?)"
        args=append(args,"%"+q+"%");
    }
    if v,ok:=filters["status"];ok&&v!=""{where+=" AND status=?";args=append(args,v)}
    rows,_:=d.db.Query(`SELECT id,name,source_url,target_url,events,status,delivery_count,last_delivery_at,created_at FROM webhooks WHERE `+where+` ORDER BY created_at DESC`,args...)
    if rows==nil{return nil};defer rows.Close()
    var o []Webhook;for rows.Next(){var e Webhook;rows.Scan(&e.ID,&e.Name,&e.SourceURL,&e.TargetURL,&e.Events,&e.Status,&e.DeliveryCount,&e.LastDeliveryAt,&e.CreatedAt);o=append(o,e)};return o
}

func(d *DB)Stats()map[string]any{
    m:=map[string]any{"total":d.Count()}
    rows,_:=d.db.Query(`SELECT status,COUNT(*) FROM webhooks GROUP BY status`)
    if rows!=nil{defer rows.Close();by:=map[string]int{};for rows.Next(){var s string;var c int;rows.Scan(&s,&c);by[s]=c};m["by_status"]=by}
    return m
}
