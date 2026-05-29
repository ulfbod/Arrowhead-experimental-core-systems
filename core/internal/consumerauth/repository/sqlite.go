package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"arrowhead/core/internal/consumerauth/model"
	_ "modernc.org/sqlite"
)

// SQLiteRepository is a persistent Repository backed by SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens (or creates) a SQLite database at dbPath and
// ensures the schema exists. Use ":memory:" for an in-process ephemeral store.
func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS auth_policies (
		instance_id         TEXT PRIMARY KEY,
		auth_level          TEXT NOT NULL DEFAULT 'PR',
		cloud               TEXT NOT NULL DEFAULT 'LOCAL',
		provider            TEXT NOT NULL DEFAULT '',
		target_type         TEXT NOT NULL DEFAULT '',
		target              TEXT NOT NULL DEFAULT '',
		description         TEXT NOT NULL DEFAULT '',
		default_policy_type TEXT NOT NULL DEFAULT '',
		default_policy_list TEXT NOT NULL DEFAULT '[]',
		scoped_policies     TEXT NOT NULL DEFAULT '{}',
		created_by          TEXT NOT NULL DEFAULT '',
		created_at          TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

// Close releases the database connection.
func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) Save(policy model.AuthPolicy) model.AuthPolicy {
	if policy.CreatedAt == "" {
		policy.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	listJSON, _ := json.Marshal(policy.DefaultPolicy.PolicyList)
	scopedJSON, _ := json.Marshal(policy.ScopedPolicies)
	r.db.Exec(`INSERT INTO auth_policies
		(instance_id, auth_level, cloud, provider, target_type, target, description,
		 default_policy_type, default_policy_list, scoped_policies, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(instance_id) DO UPDATE SET
			default_policy_type=excluded.default_policy_type,
			default_policy_list=excluded.default_policy_list,
			scoped_policies=excluded.scoped_policies,
			description=excluded.description,
			created_by=excluded.created_by`,
		policy.InstanceID, policy.AuthLevel, policy.Cloud, policy.Provider,
		policy.TargetType, policy.Target, policy.Description,
		policy.DefaultPolicy.PolicyType, string(listJSON), string(scopedJSON),
		policy.CreatedBy, policy.CreatedAt,
	)
	return policy
}

func (r *SQLiteRepository) Delete(instanceID string) bool {
	res, err := r.db.Exec(`DELETE FROM auth_policies WHERE instance_id=?`, instanceID)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (r *SQLiteRepository) FindByInstanceID(instanceID string) (model.AuthPolicy, bool) {
	row := r.db.QueryRow(
		`SELECT instance_id, auth_level, cloud, provider, target_type, target, description,
		        default_policy_type, default_policy_list, scoped_policies, created_by, created_at
		 FROM auth_policies WHERE instance_id=?`, instanceID)
	return scanPolicy(row)
}

func (r *SQLiteRepository) All() []model.AuthPolicy {
	rows, err := r.db.Query(
		`SELECT instance_id, auth_level, cloud, provider, target_type, target, description,
		        default_policy_type, default_policy_list, scoped_policies, created_by, created_at
		 FROM auth_policies ORDER BY instance_id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []model.AuthPolicy
	for rows.Next() {
		if p, ok := scanPolicy(rows); ok {
			result = append(result, p)
		}
	}
	return result
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPolicy(s scanner) (model.AuthPolicy, bool) {
	var p model.AuthPolicy
	var listJSON, scopedJSON string
	err := s.Scan(
		&p.InstanceID, &p.AuthLevel, &p.Cloud, &p.Provider,
		&p.TargetType, &p.Target, &p.Description,
		&p.DefaultPolicy.PolicyType, &listJSON, &scopedJSON,
		&p.CreatedBy, &p.CreatedAt,
	)
	if err != nil {
		return model.AuthPolicy{}, false
	}
	json.Unmarshal([]byte(listJSON), &p.DefaultPolicy.PolicyList)
	json.Unmarshal([]byte(scopedJSON), &p.ScopedPolicies)
	if p.ScopedPolicies == nil {
		p.ScopedPolicies = make(map[string]model.PolicyDef)
	}
	return p, true
}
