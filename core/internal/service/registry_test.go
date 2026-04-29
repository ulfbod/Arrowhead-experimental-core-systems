package service_test

import (
	"testing"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
)

func newService() *service.RegistryService {
	return service.NewRegistryService(repository.NewMemoryRepository())
}

func validRequest() model.RegisterRequest {
	return model.RegisterRequest{
		ServiceDefinition: "temperature-service",
		ProviderSystem: &model.System{
			SystemName: "sensor-1",
			Address:    "192.168.0.10",
			Port:       8080,
		},
		ServiceUri: "/temperature",
		Interfaces: []string{"HTTP-SECURE-JSON"},
		Version:    1,
		Metadata:   map[string]string{"region": "eu", "unit": "celsius"},
	}
}

// ---- Registration ----

func TestRegisterValid(t *testing.T) {
	svc := newService()
	got, err := svc.Register(validRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if got.ServiceDefinition != "temperature-service" {
		t.Errorf("ServiceDefinition = %q", got.ServiceDefinition)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Metadata["region"] != "eu" {
		t.Errorf("Metadata region = %q, want eu", got.Metadata["region"])
	}
}

func TestRegisterVersionDefaultsToOne(t *testing.T) {
	svc := newService()
	req := validRequest()
	req.Version = 0
	got, err := svc.Register(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
}

func TestRegisterOptionalFields(t *testing.T) {
	svc := newService()
	req := validRequest()
	req.ProviderSystem.AuthenticationInfo = "cert-fingerprint"
	req.Secure = "CERTIFICATE"
	got, err := svc.Register(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProviderSystem.AuthenticationInfo != "cert-fingerprint" {
		t.Errorf("AuthenticationInfo = %q", got.ProviderSystem.AuthenticationInfo)
	}
	if got.Secure != "CERTIFICATE" {
		t.Errorf("Secure = %q", got.Secure)
	}
}

// ---- Validation (table-driven) ----

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.RegisterRequest)
	}{
		{"missing serviceDefinition", func(r *model.RegisterRequest) { r.ServiceDefinition = "" }},
		{"whitespace serviceDefinition", func(r *model.RegisterRequest) { r.ServiceDefinition = "  " }},
		{"missing providerSystem", func(r *model.RegisterRequest) { r.ProviderSystem = nil }},
		{"missing systemName", func(r *model.RegisterRequest) { r.ProviderSystem.SystemName = "" }},
		{"missing address", func(r *model.RegisterRequest) { r.ProviderSystem.Address = "" }},
		{"port zero", func(r *model.RegisterRequest) { r.ProviderSystem.Port = 0 }},
		{"port negative", func(r *model.RegisterRequest) { r.ProviderSystem.Port = -1 }},
		{"missing serviceUri", func(r *model.RegisterRequest) { r.ServiceUri = "" }},
		{"whitespace serviceUri", func(r *model.RegisterRequest) { r.ServiceUri = "  " }},
		{"nil interfaces", func(r *model.RegisterRequest) { r.Interfaces = nil }},
		{"empty interfaces", func(r *model.RegisterRequest) { r.Interfaces = []string{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest()
			tc.mutate(&req)
			_, err := newService().Register(req)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

// ---- Duplicate / version coexistence ----

func TestRegisterDuplicateOverwrites(t *testing.T) {
	svc := newService()
	first, _ := svc.Register(validRequest())

	req := validRequest()
	req.ServiceUri = "/temperature/v2"
	req.Metadata = map[string]string{"region": "us"}
	second, err := svc.Register(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.ID != second.ID {
		t.Error("duplicate registration must keep the same ID")
	}
	if second.ServiceUri != "/temperature/v2" {
		t.Errorf("ServiceUri not updated: %q", second.ServiceUri)
	}
	if second.Metadata["region"] != "us" {
		t.Errorf("Metadata not updated: %q", second.Metadata["region"])
	}
	resp := svc.Query(model.QueryRequest{ServiceDefinition: "temperature-service"})
	if len(resp.ServiceQueryData) != 1 {
		t.Errorf("expected 1 stored entry, got %d", len(resp.ServiceQueryData))
	}
}

func TestRegisterVersionsCoexist(t *testing.T) {
	svc := newService()

	req1 := validRequest()
	req1.Version = 1
	v1, _ := svc.Register(req1)

	req2 := validRequest()
	req2.Version = 2
	v2, _ := svc.Register(req2)

	if v1.ID == v2.ID {
		t.Error("different versions must be stored as separate entries")
	}

	resp := svc.Query(model.QueryRequest{ServiceDefinition: "temperature-service"})
	if len(resp.ServiceQueryData) != 2 {
		t.Errorf("expected 2 entries (v1+v2), got %d", len(resp.ServiceQueryData))
	}
}

// ---- Unregister ----

func TestUnregisterValid(t *testing.T) {
	svc := newService()
	svc.Register(validRequest())

	err := svc.Unregister(model.UnregisterRequest{
		ServiceDefinition: "temperature-service",
		ProviderSystem:    &model.System{SystemName: "sensor-1", Address: "192.168.0.10", Port: 8080},
		Version:           1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := svc.Query(model.QueryRequest{ServiceDefinition: "temperature-service"})
	if len(resp.ServiceQueryData) != 0 {
		t.Errorf("expected 0 after unregister, got %d", len(resp.ServiceQueryData))
	}
}

func TestUnregisterNotFound(t *testing.T) {
	svc := newService()
	err := svc.Unregister(model.UnregisterRequest{
		ServiceDefinition: "nonexistent",
		ProviderSystem:    &model.System{SystemName: "s", Address: "10.0.0.1", Port: 9000},
		Version:           1,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent service")
	}
}

// ---- Query: serviceDefinition ----

func TestQueryExactMatch(t *testing.T) {
	svc := newService()
	svc.Register(validRequest())

	resp := svc.Query(model.QueryRequest{ServiceDefinition: "temperature-service"})
	if len(resp.ServiceQueryData) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.ServiceQueryData))
	}
}

func TestQueryNoMatch(t *testing.T) {
	svc := newService()
	svc.Register(validRequest())

	resp := svc.Query(model.QueryRequest{ServiceDefinition: "unknown-service"})
	if len(resp.ServiceQueryData) != 0 {
		t.Fatalf("expected 0 results, got %d", len(resp.ServiceQueryData))
	}
	if resp.UnfilteredHits != 1 {
		t.Errorf("UnfilteredHits = %d, want 1", resp.UnfilteredHits)
	}
}

func TestQueryEmptyFilterReturnsAll(t *testing.T) {
	svc := newService()
	svc.Register(validRequest())
	req2 := validRequest()
	req2.ServiceDefinition = "pressure-service"
	svc.Register(req2)

	resp := svc.Query(model.QueryRequest{})
	if len(resp.ServiceQueryData) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.ServiceQueryData))
	}
}

// ---- Query: interfaces (table-driven) ----

func TestQueryInterfaceMatching(t *testing.T) {
	tests := []struct {
		name       string
		registered []string
		query      []string
		wantMatch  bool
	}{
		{"single match", []string{"HTTP", "HTTPS"}, []string{"HTTPS"}, true},
		{"multi match", []string{"HTTP", "HTTPS"}, []string{"HTTP", "HTTPS"}, true},
		{"no match", []string{"HTTP", "HTTPS"}, []string{"COAP"}, false},
		{"partial subset no match", []string{"HTTP"}, []string{"HTTP", "HTTPS"}, false},
		{"case insensitive", []string{"HTTP-SECURE-JSON"}, []string{"http-secure-json"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService()
			req := validRequest()
			req.Interfaces = tc.registered
			svc.Register(req)

			resp := svc.Query(model.QueryRequest{Interfaces: tc.query})
			got := len(resp.ServiceQueryData) > 0
			if got != tc.wantMatch {
				t.Errorf("wantMatch=%v, got match=%v", tc.wantMatch, got)
			}
		})
	}
}

// ---- Query: metadata (table-driven) ----

func TestQueryMetadataMatching(t *testing.T) {
	tests := []struct {
		name      string
		svcMeta   map[string]string
		queryMeta map[string]string
		wantMatch bool
	}{
		{"exact match", map[string]string{"region": "eu"}, map[string]string{"region": "eu"}, true},
		{"subset match", map[string]string{"region": "eu", "unit": "celsius"}, map[string]string{"region": "eu"}, true},
		{"value mismatch", map[string]string{"region": "eu"}, map[string]string{"region": "us"}, false},
		{"key absent", map[string]string{"unit": "celsius"}, map[string]string{"region": "eu"}, false},
		{"empty query matches all", map[string]string{"region": "eu"}, nil, true},
		{"empty service meta no filter", nil, nil, true},
		{"filter on empty service meta", nil, map[string]string{"region": "eu"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService()
			req := validRequest()
			req.Metadata = tc.svcMeta
			svc.Register(req)

			resp := svc.Query(model.QueryRequest{Metadata: tc.queryMeta})
			got := len(resp.ServiceQueryData) > 0
			if got != tc.wantMatch {
				t.Errorf("wantMatch=%v, got match=%v", tc.wantMatch, got)
			}
		})
	}
}

// ---- Query: version (table-driven) ----

func TestQueryVersionRequirement(t *testing.T) {
	svc := newService()
	req1 := validRequest()
	req1.Version = 1
	svc.Register(req1)
	req2 := validRequest()
	req2.Version = 2
	svc.Register(req2)

	tests := []struct {
		name        string
		requirement int
		wantCount   int
	}{
		{"no requirement returns all", 0, 2},
		{"version 1 only", 1, 1},
		{"version 2 only", 2, 1},
		{"version 3 absent", 3, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := svc.Query(model.QueryRequest{VersionRequirement: tc.requirement})
			if len(resp.ServiceQueryData) != tc.wantCount {
				t.Errorf("wantCount=%d, got %d", tc.wantCount, len(resp.ServiceQueryData))
			}
		})
	}
}

func TestQueryVersionRequirementMatchesCorrectVersion(t *testing.T) {
	svc := newService()
	req1 := validRequest()
	req1.Version = 1
	svc.Register(req1)
	req2 := validRequest()
	req2.Version = 2
	svc.Register(req2)

	resp := svc.Query(model.QueryRequest{VersionRequirement: 2})
	if len(resp.ServiceQueryData) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.ServiceQueryData))
	}
	if resp.ServiceQueryData[0].Version != 2 {
		t.Errorf("returned version = %d, want 2", resp.ServiceQueryData[0].Version)
	}
}

// ---- Query: combined filters ----

func TestQueryCombinedFilters(t *testing.T) {
	svc := newService()

	req := validRequest()
	req.Version = 1
	req.Metadata = map[string]string{"region": "eu"}
	svc.Register(req)

	req2 := validRequest()
	req2.ProviderSystem.SystemName = "sensor-2"
	req2.Version = 1
	req2.Metadata = map[string]string{"region": "us"}
	svc.Register(req2)

	req3 := validRequest()
	req3.Version = 2
	req3.Metadata = map[string]string{"region": "eu"}
	svc.Register(req3)

	resp := svc.Query(model.QueryRequest{
		Metadata:           map[string]string{"region": "eu"},
		VersionRequirement: 1,
	})
	if len(resp.ServiceQueryData) != 1 {
		t.Errorf("expected 1, got %d", len(resp.ServiceQueryData))
	}
	if resp.UnfilteredHits != 3 {
		t.Errorf("UnfilteredHits = %d, want 3", resp.UnfilteredHits)
	}
}
