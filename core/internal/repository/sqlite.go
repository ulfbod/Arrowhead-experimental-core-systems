// Package repository — SQLite-backed Repository for the legacy ServiceRegistry.
package repository

import (
	"database/sql"
	"encoding/json"
	"sync/atomic"

	"arrowhead/core/internal/model"
	_ "modernc.org/sqlite"
)

// SQLiteRepository is a persistent Repository backed by SQLite.
type SQLiteRepository struct {
	db      *sql.DB
	counter atomic.Int64
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS service_instances (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		service_definition TEXT NOT NULL,
		system_name        TEXT NOT NULL,
		address            TEXT NOT NULL,
		port               INTEGER NOT NULL,
		version            INTEGER NOT NULL,
		service_uri        TEXT NOT NULL,
		interfaces         TEXT NOT NULL,
		metadata           TEXT NOT NULL,
		secure             TEXT NOT NULL,
		auth_info          TEXT NOT NULL,
		UNIQUE(service_definition, system_name, address, port, version)
	)`); err != nil {
		db.Close()
		return nil, err
	}
	r := &SQLiteRepository{db: db}
	// Initialise counter from current max id.
	var maxID int64
	db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM service_instances`).Scan(&maxID)
	r.counter.Store(maxID)
	return r, nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) Save(svc *model.ServiceInstance) *model.ServiceInstance {
	ifaces, _ := json.Marshal(svc.Interfaces)
	meta, _ := json.Marshal(svc.Metadata)
	secure := svc.Secure
	authInfo := svc.ProviderSystem.AuthenticationInfo

	res, err := r.db.Exec(`INSERT INTO service_instances
		(service_definition, system_name, address, port, version, service_uri, interfaces, metadata, secure, auth_info)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(service_definition, system_name, address, port, version) DO UPDATE SET
			service_uri=excluded.service_uri,
			interfaces=excluded.interfaces,
			metadata=excluded.metadata,
			secure=excluded.secure,
			auth_info=excluded.auth_info`,
		svc.ServiceDefinition, svc.ProviderSystem.SystemName, svc.ProviderSystem.Address,
		svc.ProviderSystem.Port, svc.Version, svc.ServiceUri,
		string(ifaces), string(meta), secure, authInfo,
	)
	if err != nil {
		return svc
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		// UPDATE path: fetch the existing id
		r.db.QueryRow(`SELECT id FROM service_instances WHERE service_definition=? AND system_name=? AND address=? AND port=? AND version=?`,
			svc.ServiceDefinition, svc.ProviderSystem.SystemName, svc.ProviderSystem.Address,
			svc.ProviderSystem.Port, svc.Version,
		).Scan(&id)
	}
	svc.ID = id
	return svc
}

func (r *SQLiteRepository) All() []*model.ServiceInstance {
	rows, err := r.db.Query(`SELECT id, service_definition, system_name, address, port, version, service_uri, interfaces, metadata, secure, auth_info FROM service_instances`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*model.ServiceInstance
	for rows.Next() {
		var svc model.ServiceInstance
		var ifacesJSON, metaJSON, secure, authInfo string
		if err := rows.Scan(
			&svc.ID, &svc.ServiceDefinition,
			&svc.ProviderSystem.SystemName, &svc.ProviderSystem.Address, &svc.ProviderSystem.Port,
			&svc.Version, &svc.ServiceUri, &ifacesJSON, &metaJSON, &secure, &authInfo,
		); err != nil {
			continue
		}
		json.Unmarshal([]byte(ifacesJSON), &svc.Interfaces)
		json.Unmarshal([]byte(metaJSON), &svc.Metadata)
		svc.Secure = secure
		svc.ProviderSystem.AuthenticationInfo = authInfo
		out = append(out, &svc)
	}
	return out
}

func (r *SQLiteRepository) Delete(serviceDefinition, systemName, address string, port, version int) bool {
	res, err := r.db.Exec(`DELETE FROM service_instances WHERE service_definition=? AND system_name=? AND address=? AND port=? AND version=?`,
		serviceDefinition, systemName, address, port, version,
	)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}
