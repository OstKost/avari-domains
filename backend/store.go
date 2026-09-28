package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func openStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", `CREATE TABLE IF NOT EXISTS cache (host TEXT PRIMARY KEY, payload BLOB NOT NULL, expires_at INTEGER NOT NULL)`, `CREATE TABLE IF NOT EXISTS history (id TEXT PRIMARY KEY, session TEXT NOT NULL, host TEXT NOT NULL, payload BLOB NOT NULL, created_at INTEGER NOT NULL)`, `CREATE INDEX IF NOT EXISTS history_session_date ON history(session,created_at DESC)`} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db}, nil
}
func randomID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Store) cached(host string) (*Result, bool) {
	var b []byte
	err := s.db.QueryRow("SELECT payload FROM cache WHERE host=? AND expires_at>?", host, time.Now().Unix()).Scan(&b)
	if err != nil {
		return nil, false
	}
	var r Result
	if json.Unmarshal(b, &r) != nil {
		return nil, false
	}
	r.Cached = true
	return &r, true
}
func (s *Store) putCache(r Result, ttl time.Duration) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO cache(host,payload,expires_at) VALUES(?,?,?) ON CONFLICT(host) DO UPDATE SET payload=excluded.payload,expires_at=excluded.expires_at", r.Hostname, b, time.Now().Add(ttl).Unix())
	return err
}
func (s *Store) save(session string, r Result) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO history(id,session,host,payload,created_at) VALUES(?,?,?,?,?)", r.ID, session, r.Hostname, b, time.Now().Unix())
	return err
}
func (s *Store) history(session string, page int) ([]Result, error) {
	rows, err := s.db.Query("SELECT payload FROM history WHERE session=? ORDER BY created_at DESC LIMIT 20 OFFSET ?", session, page*20)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		var b []byte
		var r Result
		if rows.Scan(&b) == nil && json.Unmarshal(b, &r) == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}
func (s *Store) item(session, id string) (Result, error) {
	var b []byte
	err := s.db.QueryRow("SELECT payload FROM history WHERE session=? AND id=?", session, id).Scan(&b)
	if err != nil {
		return Result{}, err
	}
	var r Result
	err = json.Unmarshal(b, &r)
	return r, err
}
func (s *Store) stats(session string) (map[string]int, error) {
	var total, registered, free int
	err := s.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN json_extract(payload,'$.registration.status')='registered' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN json_extract(payload,'$.registration.status')='unregistered' THEN 1 ELSE 0 END),0) FROM history WHERE session=?`, session).Scan(&total, &registered, &free)
	return map[string]int{"total": total, "registered": registered, "likelyFree": free}, err
}
func (s *Store) cleanup() {
	s.db.Exec("DELETE FROM history WHERE created_at<?", time.Now().Add(-90*24*time.Hour).Unix())
	s.db.Exec("DELETE FROM cache WHERE expires_at<?", time.Now().Unix())
}

var errBusy = errors.New("busy")
