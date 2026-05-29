// Package repository — SQLite-backed AH5StoreInterface implementation.
package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"arrowhead/core/internal/model"
	_ "modernc.org/sqlite"
)

// AH5SQLiteStore is an AH5StoreInterface backed by SQLite.
type AH5SQLiteStore struct {
	db *sql.DB
}

// NewAH5SQLiteStore opens (or creates) a SQLite database and creates the AH5 schema.
// Use ":memory:" for an in-process ephemeral store.
func NewAH5SQLiteStore(dbPath string) (*AH5SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err := createAH5Schema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &AH5SQLiteStore{db: db}, nil
}

func (s *AH5SQLiteStore) Close() error { return s.db.Close() }

func createAH5Schema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			name       TEXT PRIMARY KEY,
			metadata   TEXT NOT NULL DEFAULT '{}',
			addresses  TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS systems (
			name        TEXT PRIMARY KEY,
			device_name TEXT NOT NULL DEFAULT '',
			metadata    TEXT NOT NULL DEFAULT '{}',
			version     TEXT NOT NULL DEFAULT '',
			addresses   TEXT NOT NULL DEFAULT '[]',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS service_definitions (
			name       TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS interface_templates (
			name                  TEXT PRIMARY KEY,
			protocol              TEXT NOT NULL DEFAULT '',
			property_requirements TEXT NOT NULL DEFAULT '{}',
			created_at            TEXT NOT NULL,
			updated_at            TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ah5_service_instances (
			instance_id       TEXT PRIMARY KEY,
			system_name       TEXT NOT NULL,
			service_def_name  TEXT NOT NULL,
			version           TEXT NOT NULL,
			expires_at        TEXT NOT NULL DEFAULT '',
			metadata          TEXT NOT NULL DEFAULT '{}',
			interfaces        TEXT NOT NULL DEFAULT '[]',
			created_at        TEXT NOT NULL,
			updated_at        TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func ah5SQLiteNow() string { return time.Now().UTC().Format(time.RFC3339) }

// ─── Devices ─────────────────────────────────────────────────────────────────

func (s *AH5SQLiteStore) SaveDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool) {
	meta, _ := json.Marshal(req.Metadata)
	addrs, _ := json.Marshal(req.Addresses)
	t := ah5SQLiteNow()
	res, _ := s.db.Exec(`INSERT INTO devices (name, metadata, addresses, created_at, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET metadata=excluded.metadata, addresses=excluded.addresses, updated_at=excluded.updated_at`,
		req.Name, string(meta), string(addrs), t, t)
	created := false
	if n, _ := res.RowsAffected(); n > 0 {
		created = true
	}
	return s.GetDevice(req.Name), created
}

func (s *AH5SQLiteStore) GetDevice(name string) *model.Device {
	row := s.db.QueryRow(`SELECT name, metadata, addresses, created_at, updated_at FROM devices WHERE name=?`, name)
	d := &model.Device{}
	var meta, addrs string
	if err := row.Scan(&d.Name, &meta, &addrs, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil
	}
	json.Unmarshal([]byte(meta), &d.Metadata)
	json.Unmarshal([]byte(addrs), &d.Addresses)
	return d
}

func (s *AH5SQLiteStore) AllDevices() []*model.Device {
	rows, _ := s.db.Query(`SELECT name, metadata, addresses, created_at, updated_at FROM devices`)
	defer rows.Close()
	var out []*model.Device
	for rows.Next() {
		d := &model.Device{}
		var meta, addrs string
		if rows.Scan(&d.Name, &meta, &addrs, &d.CreatedAt, &d.UpdatedAt) == nil {
			json.Unmarshal([]byte(meta), &d.Metadata)
			json.Unmarshal([]byte(addrs), &d.Addresses)
			out = append(out, d)
		}
	}
	return out
}

func (s *AH5SQLiteStore) DeleteDevice(name string) bool {
	res, _ := s.db.Exec(`DELETE FROM devices WHERE name=?`, name)
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *AH5SQLiteStore) CreateDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool) {
	meta, _ := json.Marshal(req.Metadata)
	addrs, _ := json.Marshal(req.Addresses)
	t := ah5SQLiteNow()
	res, err := s.db.Exec(`INSERT OR IGNORE INTO devices (name, metadata, addresses, created_at, updated_at) VALUES (?,?,?,?,?)`,
		req.Name, string(meta), string(addrs), t, t)
	if err != nil {
		return nil, false
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, false
	}
	return s.GetDevice(req.Name), true
}

func (s *AH5SQLiteStore) UpdateDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool) {
	meta, _ := json.Marshal(req.Metadata)
	addrs, _ := json.Marshal(req.Addresses)
	res, _ := s.db.Exec(`UPDATE devices SET metadata=?, addresses=?, updated_at=? WHERE name=?`,
		string(meta), string(addrs), ah5SQLiteNow(), req.Name)
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, false
	}
	return s.GetDevice(req.Name), true
}

func (s *AH5SQLiteStore) HasDependentSystems(deviceName string) bool {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM systems WHERE device_name=?`, deviceName).Scan(&count)
	return count > 0
}

// ─── Systems ──────────────────────────────────────────────────────────────────

func (s *AH5SQLiteStore) SaveSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool) {
	meta, _ := json.Marshal(req.Metadata)
	addrs, _ := json.Marshal(req.Addresses)
	t := ah5SQLiteNow()
	res, _ := s.db.Exec(`INSERT INTO systems (name, device_name, metadata, version, addresses, created_at, updated_at) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET device_name=excluded.device_name, metadata=excluded.metadata, version=excluded.version, addresses=excluded.addresses, updated_at=excluded.updated_at`,
		req.Name, req.DeviceName, string(meta), req.Version, string(addrs), t, t)
	created := false
	if n, _ := res.RowsAffected(); n > 0 {
		created = true
	}
	return s.GetSystem(req.Name), created
}

func (s *AH5SQLiteStore) GetSystem(name string) *model.AH5System {
	row := s.db.QueryRow(`SELECT name, device_name, metadata, version, addresses, created_at, updated_at FROM systems WHERE name=?`, name)
	sys := &model.AH5System{}
	var meta, addrs, devName string
	if err := row.Scan(&sys.Name, &devName, &meta, &sys.Version, &addrs, &sys.CreatedAt, &sys.UpdatedAt); err != nil {
		return nil
	}
	json.Unmarshal([]byte(meta), &sys.Metadata)
	json.Unmarshal([]byte(addrs), &sys.Addresses)
	if devName != "" {
		sys.Device = s.GetDevice(devName)
	}
	return sys
}

func (s *AH5SQLiteStore) AllSystems() []*model.AH5System {
	rows, _ := s.db.Query(`SELECT name, device_name, metadata, version, addresses, created_at, updated_at FROM systems`)
	defer rows.Close()
	var out []*model.AH5System
	for rows.Next() {
		sys := &model.AH5System{}
		var meta, addrs, devName string
		if rows.Scan(&sys.Name, &devName, &meta, &sys.Version, &addrs, &sys.CreatedAt, &sys.UpdatedAt) == nil {
			json.Unmarshal([]byte(meta), &sys.Metadata)
			json.Unmarshal([]byte(addrs), &sys.Addresses)
			if devName != "" {
				sys.Device = s.GetDevice(devName)
			}
			out = append(out, sys)
		}
	}
	return out
}

func (s *AH5SQLiteStore) DeleteSystem(name string) bool {
	res, _ := s.db.Exec(`DELETE FROM systems WHERE name=?`, name)
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *AH5SQLiteStore) CreateSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool) {
	meta, _ := json.Marshal(req.Metadata)
	addrs, _ := json.Marshal(req.Addresses)
	t := ah5SQLiteNow()
	res, err := s.db.Exec(`INSERT OR IGNORE INTO systems (name, device_name, metadata, version, addresses, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		req.Name, req.DeviceName, string(meta), req.Version, string(addrs), t, t)
	if err != nil {
		return nil, false
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, false
	}
	return s.GetSystem(req.Name), true
}

func (s *AH5SQLiteStore) UpdateSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool) {
	meta, _ := json.Marshal(req.Metadata)
	addrs, _ := json.Marshal(req.Addresses)
	res, _ := s.db.Exec(`UPDATE systems SET device_name=?, metadata=?, version=?, addresses=?, updated_at=? WHERE name=?`,
		req.DeviceName, string(meta), req.Version, string(addrs), ah5SQLiteNow(), req.Name)
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, false
	}
	return s.GetSystem(req.Name), true
}

// ─── ServiceDefinitions ───────────────────────────────────────────────────────

func (s *AH5SQLiteStore) SaveServiceDefinitions(names []string) []*model.ServiceDefinition {
	t := ah5SQLiteNow()
	var out []*model.ServiceDefinition
	for _, name := range names {
		s.db.Exec(`INSERT OR IGNORE INTO service_definitions (name, created_at, updated_at) VALUES (?,?,?)`, name, t, t)
		out = append(out, s.getServiceDef(name))
	}
	return out
}

func (s *AH5SQLiteStore) CreateServiceDefinitions(names []string) ([]*model.ServiceDefinition, string) {
	for _, name := range names {
		var count int
		s.db.QueryRow(`SELECT COUNT(*) FROM service_definitions WHERE name=?`, name).Scan(&count)
		if count > 0 {
			return nil, name
		}
	}
	t := ah5SQLiteNow()
	var out []*model.ServiceDefinition
	for _, name := range names {
		s.db.Exec(`INSERT INTO service_definitions (name, created_at, updated_at) VALUES (?,?,?)`, name, t, t)
		out = append(out, s.getServiceDef(name))
	}
	return out, ""
}

func (s *AH5SQLiteStore) AllServiceDefinitions() []*model.ServiceDefinition {
	rows, _ := s.db.Query(`SELECT name, created_at, updated_at FROM service_definitions`)
	defer rows.Close()
	var out []*model.ServiceDefinition
	for rows.Next() {
		def := &model.ServiceDefinition{}
		if rows.Scan(&def.Name, &def.CreatedAt, &def.UpdatedAt) == nil {
			out = append(out, def)
		}
	}
	return out
}

func (s *AH5SQLiteStore) DeleteServiceDefinitions(names []string) {
	for _, name := range names {
		s.db.Exec(`DELETE FROM service_definitions WHERE name=?`, name)
	}
}

func (s *AH5SQLiteStore) getServiceDef(name string) *model.ServiceDefinition {
	def := &model.ServiceDefinition{}
	s.db.QueryRow(`SELECT name, created_at, updated_at FROM service_definitions WHERE name=?`, name).
		Scan(&def.Name, &def.CreatedAt, &def.UpdatedAt)
	return def
}

// ─── InterfaceTemplates ───────────────────────────────────────────────────────

func (s *AH5SQLiteStore) CreateInterfaceTemplates(templates []*model.InterfaceTemplate) ([]*model.InterfaceTemplate, string) {
	for _, tmpl := range templates {
		var count int
		s.db.QueryRow(`SELECT COUNT(*) FROM interface_templates WHERE name=?`, tmpl.Name).Scan(&count)
		if count > 0 {
			return nil, tmpl.Name
		}
	}
	t := ah5SQLiteNow()
	var out []*model.InterfaceTemplate
	for _, tmpl := range templates {
		pr, _ := json.Marshal(tmpl.PropertyRequirements)
		s.db.Exec(`INSERT INTO interface_templates (name, protocol, property_requirements, created_at, updated_at) VALUES (?,?,?,?,?)`,
			tmpl.Name, tmpl.Protocol, string(pr), t, t)
		stored := &model.InterfaceTemplate{Name: tmpl.Name, Protocol: tmpl.Protocol, PropertyRequirements: tmpl.PropertyRequirements, CreatedAt: t, UpdatedAt: t}
		out = append(out, stored)
	}
	return out, ""
}

func (s *AH5SQLiteStore) AllInterfaceTemplates() []*model.InterfaceTemplate {
	rows, _ := s.db.Query(`SELECT name, protocol, property_requirements, created_at, updated_at FROM interface_templates`)
	defer rows.Close()
	var out []*model.InterfaceTemplate
	for rows.Next() {
		tmpl := &model.InterfaceTemplate{}
		var prJSON string
		if rows.Scan(&tmpl.Name, &tmpl.Protocol, &prJSON, &tmpl.CreatedAt, &tmpl.UpdatedAt) == nil {
			json.Unmarshal([]byte(prJSON), &tmpl.PropertyRequirements)
			out = append(out, tmpl)
		}
	}
	return out
}

func (s *AH5SQLiteStore) DeleteInterfaceTemplates(names []string) {
	for _, name := range names {
		s.db.Exec(`DELETE FROM interface_templates WHERE name=?`, name)
	}
}

// ─── ServiceInstances ─────────────────────────────────────────────────────────

func (s *AH5SQLiteStore) SaveServiceInstance(req *model.ServiceRegistrationRequest) (*model.AH5ServiceInstance, bool) {
	id := compositeServiceID(req.SystemName, req.ServiceDefinitionName, req.Version)
	meta, _ := json.Marshal(req.Metadata)
	ifaces, _ := json.Marshal(req.Interfaces)
	t := ah5SQLiteNow()
	res, _ := s.db.Exec(`INSERT INTO ah5_service_instances
		(instance_id, system_name, service_def_name, version, expires_at, metadata, interfaces, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(instance_id) DO UPDATE SET expires_at=excluded.expires_at, metadata=excluded.metadata, interfaces=excluded.interfaces, updated_at=excluded.updated_at`,
		id, req.SystemName, req.ServiceDefinitionName, req.Version, req.ExpiresAt, string(meta), string(ifaces), t, t)
	created := false
	if n, _ := res.RowsAffected(); n > 0 {
		created = true
	}
	return s.getServiceInstance(id), created
}

func (s *AH5SQLiteStore) CreateServiceInstance(req *model.ServiceCreateRequest) (*model.AH5ServiceInstance, bool) {
	id := compositeServiceID(req.SystemName, req.ServiceDefinitionName, req.Version)
	meta, _ := json.Marshal(req.Metadata)
	ifaces, _ := json.Marshal(req.Interfaces)
	t := ah5SQLiteNow()
	res, err := s.db.Exec(`INSERT OR IGNORE INTO ah5_service_instances
		(instance_id, system_name, service_def_name, version, expires_at, metadata, interfaces, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, req.SystemName, req.ServiceDefinitionName, req.Version, req.ExpiresAt, string(meta), string(ifaces), t, t)
	if err != nil {
		return nil, false
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, false
	}
	return s.getServiceInstance(id), true
}

func (s *AH5SQLiteStore) UpdateServiceInstance(req *model.ServiceUpdateRequest) (*model.AH5ServiceInstance, bool) {
	meta, _ := json.Marshal(req.Metadata)
	ifaces, _ := json.Marshal(req.Interfaces)
	res, _ := s.db.Exec(`UPDATE ah5_service_instances SET expires_at=?, metadata=?, interfaces=?, updated_at=? WHERE instance_id=?`,
		req.ExpiresAt, string(meta), string(ifaces), ah5SQLiteNow(), req.InstanceID)
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, false
	}
	return s.getServiceInstance(req.InstanceID), true
}

func (s *AH5SQLiteStore) AllServiceInstances() []*model.AH5ServiceInstance {
	rows, _ := s.db.Query(`SELECT instance_id, system_name, service_def_name, version, expires_at, metadata, interfaces, created_at, updated_at FROM ah5_service_instances`)
	defer rows.Close()
	var out []*model.AH5ServiceInstance
	for rows.Next() {
		inst := &model.AH5ServiceInstance{}
		var metaJSON, ifacesJSON string
		var sysName string
		if rows.Scan(&inst.InstanceID, &sysName, &inst.ServiceDefinitionName, &inst.Version, &inst.ExpiresAt, &metaJSON, &ifacesJSON, &inst.CreatedAt, &inst.UpdatedAt) == nil {
			json.Unmarshal([]byte(metaJSON), &inst.Metadata)
			json.Unmarshal([]byte(ifacesJSON), &inst.Interfaces)
			inst.Provider = s.GetSystem(sysName)
			if inst.Provider == nil {
				inst.Provider = &model.AH5System{Name: sysName}
			}
			out = append(out, inst)
		}
	}
	return out
}

func (s *AH5SQLiteStore) DeleteServiceInstance(id string) bool {
	res, _ := s.db.Exec(`DELETE FROM ah5_service_instances WHERE instance_id=?`, id)
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *AH5SQLiteStore) DeleteServiceInstances(ids []string) {
	for _, id := range ids {
		s.db.Exec(`DELETE FROM ah5_service_instances WHERE instance_id=?`, id)
	}
}

func (s *AH5SQLiteStore) getServiceInstance(id string) *model.AH5ServiceInstance {
	row := s.db.QueryRow(`SELECT instance_id, system_name, service_def_name, version, expires_at, metadata, interfaces, created_at, updated_at FROM ah5_service_instances WHERE instance_id=?`, id)
	inst := &model.AH5ServiceInstance{}
	var metaJSON, ifacesJSON, sysName string
	if err := row.Scan(&inst.InstanceID, &sysName, &inst.ServiceDefinitionName, &inst.Version, &inst.ExpiresAt, &metaJSON, &ifacesJSON, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
		return nil
	}
	json.Unmarshal([]byte(metaJSON), &inst.Metadata)
	json.Unmarshal([]byte(ifacesJSON), &inst.Interfaces)
	inst.Provider = s.GetSystem(sysName)
	if inst.Provider == nil {
		inst.Provider = &model.AH5System{Name: sysName}
	}
	return inst
}
