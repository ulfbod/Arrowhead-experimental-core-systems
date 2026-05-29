package repository

import (
	"database/sql"
	"encoding/json"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS flexible_rules (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		consumer_system_name TEXT NOT NULL,
		service_definition   TEXT NOT NULL,
		provider_name        TEXT NOT NULL,
		provider_address     TEXT NOT NULL,
		provider_port        INTEGER NOT NULL,
		service_uri          TEXT NOT NULL,
		interfaces           TEXT NOT NULL,
		priority             INTEGER NOT NULL DEFAULT 0,
		metadata_filter      TEXT NOT NULL,
		metadata             TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) Save(rule model.FlexibleRule) model.FlexibleRule {
	ifaces, _ := json.Marshal(rule.Interfaces)
	mf, _ := json.Marshal(rule.MetadataFilter)
	res, err := r.db.Exec(
		`INSERT INTO flexible_rules (consumer_system_name, service_definition, provider_name, provider_address, provider_port, service_uri, interfaces, priority, metadata_filter, metadata) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		rule.ConsumerSystemName, rule.ServiceDefinition,
		rule.Provider.SystemName, rule.Provider.Address, rule.Provider.Port,
		rule.ServiceUri, string(ifaces), rule.Priority, string(mf), "{}",
	)
	if err != nil {
		return rule
	}
	rule.ID, _ = res.LastInsertId()
	return rule
}

func (r *SQLiteRepository) Delete(id int64) bool {
	res, err := r.db.Exec(`DELETE FROM flexible_rules WHERE id=?`, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (r *SQLiteRepository) All() []model.FlexibleRule {
	rows, err := r.db.Query(
		`SELECT id, consumer_system_name, service_definition, provider_name, provider_address, provider_port, service_uri, interfaces, priority, metadata_filter, metadata FROM flexible_rules ORDER BY id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var rules []model.FlexibleRule
	for rows.Next() {
		var rule model.FlexibleRule
		var ifacesJSON, mfJSON string
		var provName, provAddr string
		var provPort int
		var unused string
		if err := rows.Scan(
			&rule.ID, &rule.ConsumerSystemName, &rule.ServiceDefinition,
			&provName, &provAddr, &provPort,
			&rule.ServiceUri, &ifacesJSON, &rule.Priority, &mfJSON, &unused,
		); err != nil {
			continue
		}
		json.Unmarshal([]byte(ifacesJSON), &rule.Interfaces)
		json.Unmarshal([]byte(mfJSON), &rule.MetadataFilter)
		rule.Provider = orchmodel.System{SystemName: provName, Address: provAddr, Port: provPort}
		rules = append(rules, rule)
	}
	return rules
}
