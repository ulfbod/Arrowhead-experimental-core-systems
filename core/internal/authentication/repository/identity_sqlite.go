package repository

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteIdentityRepository is a persistent identity store backed by SQLite.
type SQLiteIdentityRepository struct {
	db *sql.DB
}

func NewSQLiteIdentityRepository(dbPath string) (*SQLiteIdentityRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS identities (
		system_name   TEXT PRIMARY KEY,
		password_hash TEXT NOT NULL DEFAULT '',
		sysop         INTEGER NOT NULL DEFAULT 0,
		created_by    TEXT NOT NULL DEFAULT '',
		created_at    TEXT NOT NULL DEFAULT '',
		updated_at    TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteIdentityRepository{db: db}, nil
}

func (r *SQLiteIdentityRepository) Close() error { return r.db.Close() }

func (r *SQLiteIdentityRepository) Save(id Identity) {
	now := time.Now().UTC().Format(time.RFC3339)
	if id.CreatedAt == "" {
		id.CreatedAt = now
	}
	id.UpdatedAt = now
	sysop := 0
	if id.Sysop {
		sysop = 1
	}
	r.db.Exec(`INSERT INTO identities (system_name, password_hash, sysop, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(system_name) DO UPDATE SET
			password_hash=excluded.password_hash,
			sysop=excluded.sysop,
			created_by=excluded.created_by,
			updated_at=excluded.updated_at`,
		id.SystemName, id.PasswordHash, sysop, id.CreatedBy, id.CreatedAt, id.UpdatedAt,
	)
}

func (r *SQLiteIdentityRepository) Get(systemName string) (Identity, bool) {
	row := r.db.QueryRow(
		`SELECT system_name, password_hash, sysop, created_by, created_at, updated_at FROM identities WHERE system_name=?`,
		systemName,
	)
	var id Identity
	var sysopInt int
	if err := row.Scan(&id.SystemName, &id.PasswordHash, &sysopInt, &id.CreatedBy, &id.CreatedAt, &id.UpdatedAt); err != nil {
		return Identity{}, false
	}
	id.Sysop = sysopInt != 0
	return id, true
}

func (r *SQLiteIdentityRepository) Delete(systemName string) {
	r.db.Exec(`DELETE FROM identities WHERE system_name=?`, systemName)
}

func (r *SQLiteIdentityRepository) All() []Identity {
	rows, err := r.db.Query(`SELECT system_name, password_hash, sysop, created_by, created_at, updated_at FROM identities`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Identity
	for rows.Next() {
		var id Identity
		var sysopInt int
		if err := rows.Scan(&id.SystemName, &id.PasswordHash, &sysopInt, &id.CreatedBy, &id.CreatedAt, &id.UpdatedAt); err != nil {
			continue
		}
		id.Sysop = sysopInt != 0
		result = append(result, id)
	}
	return result
}
