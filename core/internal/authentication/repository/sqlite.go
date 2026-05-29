package repository

import (
	"database/sql"
	"time"

	"arrowhead/core/internal/authentication/model"
	_ "modernc.org/sqlite"
)

// SQLiteRepository is a persistent token store backed by SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS identity_tokens (
		token       TEXT PRIMARY KEY,
		system_name TEXT NOT NULL,
		expires_at  TEXT NOT NULL,
		login_time  TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) Save(t *model.IdentityToken) {
	r.db.Exec(
		`INSERT OR REPLACE INTO identity_tokens (token, system_name, expires_at, login_time) VALUES (?,?,?,?)`,
		t.Token, t.SystemName,
		t.ExpiresAt.UTC().Format(time.RFC3339),
		t.LoginTime.UTC().Format(time.RFC3339),
	)
}

func (r *SQLiteRepository) FindByToken(token string) (*model.IdentityToken, bool) {
	row := r.db.QueryRow(
		`SELECT token, system_name, expires_at, login_time FROM identity_tokens WHERE token=?`, token,
	)
	var t model.IdentityToken
	var expiresAtStr, loginTimeStr string
	if err := row.Scan(&t.Token, &t.SystemName, &expiresAtStr, &loginTimeStr); err != nil {
		return nil, false
	}
	t.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
	t.LoginTime, _ = time.Parse(time.RFC3339, loginTimeStr)
	return &t, true
}

func (r *SQLiteRepository) FindBySystemName(name string) (*model.IdentityToken, bool) {
	row := r.db.QueryRow(
		`SELECT token, system_name, expires_at, login_time FROM identity_tokens WHERE system_name=? AND expires_at > ? LIMIT 1`,
		name, time.Now().UTC().Format(time.RFC3339),
	)
	var t model.IdentityToken
	var expiresAtStr, loginTimeStr string
	if err := row.Scan(&t.Token, &t.SystemName, &expiresAtStr, &loginTimeStr); err != nil {
		return nil, false
	}
	t.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
	t.LoginTime, _ = time.Parse(time.RFC3339, loginTimeStr)
	return &t, true
}

func (r *SQLiteRepository) Delete(token string) bool {
	res, err := r.db.Exec(`DELETE FROM identity_tokens WHERE token=?`, token)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (r *SQLiteRepository) DeleteBySystemName(name string) {
	r.db.Exec(`DELETE FROM identity_tokens WHERE system_name=?`, name)
}

func (r *SQLiteRepository) All() []*model.IdentityToken {
	rows, err := r.db.Query(`SELECT token, system_name, expires_at, login_time FROM identity_tokens`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*model.IdentityToken
	for rows.Next() {
		var t model.IdentityToken
		var expiresAtStr, loginTimeStr string
		if err := rows.Scan(&t.Token, &t.SystemName, &expiresAtStr, &loginTimeStr); err != nil {
			continue
		}
		t.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
		t.LoginTime, _ = time.Parse(time.RFC3339, loginTimeStr)
		result = append(result, &t)
	}
	return result
}

func (r *SQLiteRepository) DeleteExpired() {
	r.db.Exec(`DELETE FROM identity_tokens WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339))
}
