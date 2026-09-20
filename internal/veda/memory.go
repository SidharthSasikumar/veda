package veda

import (
	"database/sql"
	"encoding/json"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path/filepath"
	"time"
)

type Store struct{ DB *sql.DB }
type Run struct {
	ID          string       `json:"id"`
	Objective   string       `json:"objective"`
	Status      string       `json:"status"`
	Stage       string       `json:"stage"`
	Mode        string       `json:"mode"`
	Created     string       `json:"created"`
	Finished    string       `json:"finished,omitempty"`
	Error       string       `json:"error,omitempty"`
	Best        string       `json:"best,omitempty"`
	Improvement float64      `json:"improvement"`
	Config      Config       `json:"config"`
	Baseline    *Measurement `json:"baseline,omitempty"`
	Experiments []Experiment `json:"experiments"`
	Report      string       `json:"report,omitempty"`
}
type Experiment struct {
	ID          string       `json:"id"`
	RunID       string       `json:"run_id"`
	Hypothesis  string       `json:"hypothesis"`
	Rationale   string       `json:"rationale"`
	Status      string       `json:"status"`
	Measurement *Measurement `json:"measurement,omitempty"`
	Improvement float64      `json:"improvement"`
	Error       string       `json:"error,omitempty"`
	Path        string       `json:"path"`
}
type Event struct {
	ID          int64         `json:"id"`
	RunID       string        `json:"run_id"`
	At          string        `json:"at"`
	Message     string        `json:"message"`
	Version     int           `json:"version,omitempty"`
	Type        string        `json:"type,omitempty"`
	Stage       string        `json:"stage,omitempty"`
	Actor       string        `json:"actor_id,omitempty"`
	Recipient   string        `json:"recipient_id,omitempty"`
	TaskID      string        `json:"task_id,omitempty"`
	ArtifactIDs []string      `json:"artifact_ids,omitempty"`
	Status      string        `json:"status,omitempty"`
	Task        *WorkflowTask `json:"task,omitempty"`
}

func OpenStore(root string) (*Store, error) {
	d := filepath.Join(root, ".veda")
	if e := os.MkdirAll(d, 0700); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite3", filepath.Join(d, "memory.db")+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY, created TEXT NOT NULL, data TEXT NOT NULL); CREATE TABLE IF NOT EXISTS experiments(id TEXT PRIMARY KEY,run_id TEXT NOT NULL REFERENCES runs(id),data TEXT NOT NULL); CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY AUTOINCREMENT,run_id TEXT NOT NULL,at TEXT NOT NULL,message TEXT NOT NULL);`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Store{db}, nil
}
func (s *Store) SaveRun(r *Run) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("INSERT INTO runs VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", r.ID, r.Created, string(b))
	return e
}
func (s *Store) SaveExperiment(e Experiment) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO experiments VALUES(?,?,?)", e.RunID+"/"+e.ID, e.RunID, string(b))
	return err
}
func (s *Store) Event(id, msg string) error {
	_, e := s.DB.Exec("INSERT INTO events(run_id,at,message) VALUES(?,?,?)", id, time.Now().UTC().Format(time.RFC3339), msg)
	return e
}
func (s *Store) Runs() ([]Run, error) {
	rows, e := s.DB.Query("SELECT data FROM runs ORDER BY created DESC LIMIT 100")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var b string
		var r Run
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) Get(id string) (Run, error) {
	var r Run
	var b string
	e := s.DB.QueryRow("SELECT data FROM runs WHERE id=?", id).Scan(&b)
	if e != nil {
		return r, e
	}
	e = json.Unmarshal([]byte(b), &r)
	return r, e
}
func (s *Store) Events(id string) ([]Event, error) {
	rows, e := s.DB.Query("SELECT id,run_id,at,message FROM events WHERE run_id=? ORDER BY id", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var v Event
		if e = rows.Scan(&v.ID, &v.RunID, &v.At, &v.Message); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) History() string {
	rows, e := s.DB.Query("SELECT data FROM experiments ORDER BY rowid DESC LIMIT 12")
	if e != nil {
		return ""
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var b string
		rows.Scan(&b)
		out += b + "\n"
	}
	if len(out) > 6000 {
		out = out[:6000]
	}
	return out
}
