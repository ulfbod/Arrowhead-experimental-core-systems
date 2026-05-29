package repository

import (
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteRepository is a persistent CA repository backed by SQLite.
type SQLiteRepository struct {
	db     *sql.DB
	serial atomic.Int64
	mu     sync.Mutex
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS revocations (
		serial      TEXT PRIMARY KEY,
		system_name TEXT NOT NULL,
		revoked_at  TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ca_state (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}

	r := &SQLiteRepository{db: db}
	// Load persisted serial counter (default 2 — 1 is the CA root cert).
	var val string
	if err := db.QueryRow(`SELECT value FROM ca_state WHERE key='next_serial'`).Scan(&val); err == nil {
		var n int64
		if _, err := fmt.Sscan(val, &n); err == nil {
			r.serial.Store(n)
		}
	} else {
		r.serial.Store(2)
		db.Exec(`INSERT OR IGNORE INTO ca_state (key, value) VALUES ('next_serial','2')`)
	}
	return r, nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) NextSerial() int64 { return r.serial.Load() }

func (r *SQLiteRepository) IncrementSerial() int64 {
	n := r.serial.Add(1)
	r.db.Exec(`INSERT OR REPLACE INTO ca_state (key, value) VALUES ('next_serial',?)`, fmt.Sprint(n))
	return n
}

func (r *SQLiteRepository) AddRevocation(serial, systemName string, revokedAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.db.Exec(`INSERT OR IGNORE INTO revocations (serial, system_name, revoked_at) VALUES (?,?,?)`,
		serial, systemName, revokedAt.UTC().Format(time.RFC3339))
}

func (r *SQLiteRepository) IsRevoked(serial string) bool {
	var count int
	r.db.QueryRow(`SELECT COUNT(*) FROM revocations WHERE serial=?`, serial).Scan(&count)
	return count > 0
}

func (r *SQLiteRepository) AllRevocations() []Revocation {
	rows, err := r.db.Query(`SELECT serial, system_name, revoked_at FROM revocations ORDER BY revoked_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Revocation
	for rows.Next() {
		var rev Revocation
		var revokedAtStr string
		if rows.Scan(&rev.Serial, &rev.SystemName, &revokedAtStr) == nil {
			rev.RevokedAt, _ = time.Parse(time.RFC3339, revokedAtStr)
			out = append(out, rev)
		}
	}
	return out
}
