package repository

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteRepository is a persistent Repository backed by SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS blacklist_entries (
		system_name TEXT NOT NULL,
		reason      TEXT NOT NULL,
		expires_at  TEXT,
		active      INTEGER NOT NULL DEFAULT 1,
		created_by  TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) Save(e Entry) Entry {
	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now

	var expiresAt sql.NullString
	if !e.ExpiresAt.IsZero() {
		expiresAt = sql.NullString{String: e.ExpiresAt.Format(time.RFC3339), Valid: true}
	}
	active := 0
	if e.Active {
		active = 1
	}
	r.db.Exec(
		`INSERT INTO blacklist_entries (system_name, reason, expires_at, active, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		e.SystemName, e.Reason, expiresAt, active, e.CreatedBy,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	return e
}

func (r *SQLiteRepository) All() []Entry {
	rows, err := r.db.Query(
		`SELECT system_name, reason, expires_at, active, created_by, created_at, updated_at FROM blacklist_entries ORDER BY rowid`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		var expiresAt sql.NullString
		var active int
		var createdAt, updatedAt string
		if err := rows.Scan(&e.SystemName, &e.Reason, &expiresAt, &active, &e.CreatedBy, &createdAt, &updatedAt); err != nil {
			continue
		}
		e.Active = active != 0
		if expiresAt.Valid {
			if t, err := time.Parse(time.RFC3339, expiresAt.String); err == nil {
				e.ExpiresAt = t
			}
		}
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			e.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			e.UpdatedAt = t
		}
		entries = append(entries, e)
	}
	return entries
}

func (r *SQLiteRepository) SetActive(systemName string, active bool) bool {
	activeVal := 0
	if active {
		activeVal = 1
	}
	res, err := r.db.Exec(
		`UPDATE blacklist_entries SET active=?, updated_at=? WHERE system_name=?`,
		activeVal, time.Now().UTC().Format(time.RFC3339), systemName,
	)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}
