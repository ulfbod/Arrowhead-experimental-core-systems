package service_test

import (
	"testing"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/repository"
	"arrowhead/core/internal/orchestration/simplestore/service"
)

func newOrchestrator() *service.SimpleStoreOrchestrator {
	return service.NewSimpleStoreOrchestrator(repository.NewMemoryRepository())
}

func validCreateRule() model.CreateRuleRequest {
	return model.CreateRuleRequest{
		ConsumerSystemName: "consumer-app",
		ServiceDefinition:  "temperature-service",
		Provider: orchmodel.System{
			SystemName: "sensor-1",
			Address:    "10.0.0.1",
			Port:       9000,
		},
		ServiceUri: "/temperature",
		Interfaces: []string{"HTTP-INSECURE-JSON"},
	}
}

// ---- CreateRule ----

func TestCreateRuleValid(t *testing.T) {
	orch := newOrchestrator()
	rule, err := orch.CreateRule(validCreateRule())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if rule.ConsumerSystemName != "consumer-app" {
		t.Errorf("ConsumerSystemName = %q", rule.ConsumerSystemName)
	}
}

func TestCreateRuleValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.CreateRuleRequest)
	}{
		{"empty consumer", func(r *model.CreateRuleRequest) { r.ConsumerSystemName = "" }},
		{"whitespace consumer", func(r *model.CreateRuleRequest) { r.ConsumerSystemName = "  " }},
		{"empty service", func(r *model.CreateRuleRequest) { r.ServiceDefinition = "" }},
		{"empty provider name", func(r *model.CreateRuleRequest) { r.Provider.SystemName = "" }},
		{"empty serviceUri", func(r *model.CreateRuleRequest) { r.ServiceUri = "" }},
		{"empty interfaces", func(r *model.CreateRuleRequest) { r.Interfaces = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validCreateRule()
			tc.mutate(&req)
			_, err := newOrchestrator().CreateRule(req)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// ---- DeleteRule ----

func TestDeleteRuleValid(t *testing.T) {
	orch := newOrchestrator()
	rule, _ := orch.CreateRule(validCreateRule())
	if err := orch.DeleteRule(rule.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := orch.ListRules()
	if resp.Count != 0 {
		t.Errorf("expected 0 rules after delete, got %d", resp.Count)
	}
}

func TestDeleteRuleNotFound(t *testing.T) {
	orch := newOrchestrator()
	if err := orch.DeleteRule(999); err == nil {
		t.Fatal("expected error for nonexistent rule")
	}
}

// ---- ListRules ----

func TestListRulesEmpty(t *testing.T) {
	orch := newOrchestrator()
	resp := orch.ListRules()
	if resp.Rules == nil {
		t.Error("expected non-nil slice")
	}
	if resp.Count != 0 {
		t.Errorf("expected 0, got %d", resp.Count)
	}
}

func TestListRulesReturnsAll(t *testing.T) {
	orch := newOrchestrator()
	orch.CreateRule(validCreateRule())
	req2 := validCreateRule()
	req2.ConsumerSystemName = "other-consumer"
	orch.CreateRule(req2)

	resp := orch.ListRules()
	if resp.Count != 2 {
		t.Errorf("expected 2, got %d", resp.Count)
	}
}

// ---- Orchestrate ----

func TestOrchestrateMissingRequester(t *testing.T) {
	orch := newOrchestrator()
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	})
	if err == nil {
		t.Fatal("expected error for missing requesterSystem.systemName")
	}
}

func TestOrchestrateMissingService(t *testing.T) {
	orch := newOrchestrator()
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem: orchmodel.System{SystemName: "consumer-app"},
	})
	if err == nil {
		t.Fatal("expected error for missing serviceDefinition")
	}
}

func TestOrchestrateMatch(t *testing.T) {
	orch := newOrchestrator()
	orch.CreateRule(validCreateRule())

	resp, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app"},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "sensor-1" {
		t.Errorf("provider = %q", resp.Response[0].Provider.SystemName)
	}
	if resp.Response[0].Service.ServiceUri != "/temperature" {
		t.Errorf("serviceUri = %q", resp.Response[0].Service.ServiceUri)
	}
}

func TestOrchestrateNoMatch(t *testing.T) {
	orch := newOrchestrator()
	orch.CreateRule(validCreateRule())

	resp, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app"},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "unknown-service"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected empty response, got %d", len(resp.Response))
	}
}

func TestOrchestrateWrongConsumer(t *testing.T) {
	orch := newOrchestrator()
	orch.CreateRule(validCreateRule())

	resp, _ := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "wrong-consumer"},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	})
	if len(resp.Response) != 0 {
		t.Error("expected no match for wrong consumer")
	}
}

func TestOrchestrateMultipleRulesReturnsAll(t *testing.T) {
	orch := newOrchestrator()
	orch.CreateRule(validCreateRule())
	req2 := validCreateRule()
	req2.Provider.SystemName = "sensor-2"
	req2.Provider.Address = "10.0.0.2"
	orch.CreateRule(req2)

	resp, _ := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app"},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	})
	if len(resp.Response) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Response))
	}
}
