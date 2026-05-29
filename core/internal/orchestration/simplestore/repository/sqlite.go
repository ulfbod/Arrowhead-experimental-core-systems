package repository

import (
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS store_rules (
		id                  TEXT PRIMARY KEY,
		consumer_system_name TEXT NOT NULL,
		service_definition   TEXT NOT NULL,
		provider_name        TEXT NOT NULL,
		provider_address     TEXT NOT NULL,
		provider_port        INTEGER NOT NULL,
		service_uri          TEXT NOT NULL,
		interfaces           TEXT NOT NULL,
		metadata             TEXT NOT NULL,
		priority             INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) Save(rule model.StoreRule) model.StoreRule {
	rule.ID = uuid.NewString()
	ifaces, _ := json.Marshal(rule.Interfaces)
	meta, _ := json.Marshal(rule.Metadata)
	_, err := r.db.Exec(
		`INSERT INTO store_rules (id, consumer_system_name, service_definition, provider_name, provider_address, provider_port, service_uri, interfaces, metadata, priority) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		rule.ID, rule.ConsumerSystemName, rule.ServiceDefinition,
		rule.Provider.SystemName, rule.Provider.Address, rule.Provider.Port,
		rule.ServiceUri, string(ifaces), string(meta), rule.Priority,
	)
	if err != nil {
		return rule
	}
	return rule
}

func (r *SQLiteRepository) Delete(id string) bool {
	res, err := r.db.Exec(`DELETE FROM store_rules WHERE id=?`, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (r *SQLiteRepository) UpdatePriority(id string, priority int) bool {
	res, err := r.db.Exec(`UPDATE store_rules SET priority=? WHERE id=?`, priority, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (r *SQLiteRepository) All() []model.StoreRule {
	rows, err := r.db.Query(
		`SELECT id, consumer_system_name, service_definition, provider_name, provider_address, provider_port, service_uri, interfaces, metadata, priority FROM store_rules ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var rules []model.StoreRule
	for rows.Next() {
		var rule model.StoreRule
		var ifacesJSON, metaJSON string
		if err := rows.Scan(
			&rule.ID, &rule.ConsumerSystemName, &rule.ServiceDefinition,
			&rule.Provider.SystemName, &rule.Provider.Address, &rule.Provider.Port,
			&rule.ServiceUri, &ifacesJSON, &metaJSON, &rule.Priority,
		); err != nil {
			continue
		}
		json.Unmarshal([]byte(ifacesJSON), &rule.Interfaces)
		json.Unmarshal([]byte(metaJSON), &rule.Metadata)
		rule.Provider = orchmodel.System{
			SystemName: rule.Provider.SystemName,
			Address:    rule.Provider.Address,
			Port:       rule.Provider.Port,
		}
		rules = append(rules, rule)
	}
	return rules
}
