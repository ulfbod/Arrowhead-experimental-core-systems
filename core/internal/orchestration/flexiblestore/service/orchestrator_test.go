package service_test

import (
	"testing"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
	"arrowhead/core/internal/orchestration/flexiblestore/repository"
	"arrowhead/core/internal/orchestration/flexiblestore/service"
)

func newOrchestrator() *service.FlexibleStoreOrchestrator {
	return service.NewFlexibleStoreOrchestrator(repository.NewMemoryRepository())
}

func validCreateRule() model.CreateFlexibleRuleRequest {
	return model.CreateFlexibleRuleRequest{
		ConsumerSystemName: "consumer-app",
		ServiceDefinition:  "temperature-service",
		Provider: orchmodel.System{
			SystemName: "sensor-1",
			Address:    "10.0.0.1",
			Port:       9000,
		},
		ServiceUri: "/temperature",
		Interfaces: []string{"HTTP-INSECURE-JSON"},
		Priority:   1,
	}
}

func validOrchRequest() orchmodel.OrchestrationRequest {
	return orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app"},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
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
	if rule.Priority != 1 {
		t.Errorf("Priority = %d, want 1", rule.Priority)
	}
}

func TestCreateRuleValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.CreateFlexibleRuleRequest)
	}{
		{"empty consumer", func(r *model.CreateFlexibleRuleRequest) { r.ConsumerSystemName = "" }},
		{"empty service", func(r *model.CreateFlexibleRuleRequest) { r.ServiceDefinition = "" }},
		{"empty provider name", func(r *model.CreateFlexibleRuleRequest) { r.Provider.SystemName = "" }},
		{"empty serviceUri", func(r *model.CreateFlexibleRuleRequest) { r.ServiceUri = "" }},
		{"empty interfaces", func(r *model.CreateFlexibleRuleRequest) { r.Interfaces = nil }},
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

// ---- Priority ordering ----

func TestOrchestratePriorityOrdering(t *testing.T) {
	orch := newOrchestrator()

	// Insert in reverse priority order to prove sorting works.
	req3 := validCreateRule()
	req3.Priority = 3
	req3.Provider.SystemName = "sensor-low"
	orch.CreateRule(req3)

	req1 := validCreateRule()
	req1.Priority = 1
	req1.Provider.SystemName = "sensor-high"
	orch.CreateRule(req1)

	req2 := validCreateRule()
	req2.Priority = 2
	req2.Provider.SystemName = "sensor-mid"
	orch.CreateRule(req2)

	resp, err := orch.Orchestrate(validOrchRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "sensor-high" {
		t.Errorf("first result should be highest priority (sensor-high), got %q", resp.Response[0].Provider.SystemName)
	}
	if resp.Response[1].Provider.SystemName != "sensor-mid" {
		t.Errorf("second result should be sensor-mid, got %q", resp.Response[1].Provider.SystemName)
	}
	if resp.Response[2].Provider.SystemName != "sensor-low" {
		t.Errorf("third result should be sensor-low, got %q", resp.Response[2].Provider.SystemName)
	}
}

func TestOrchestratePriorityZeroIsLowest(t *testing.T) {
	orch := newOrchestrator()

	explicit := validCreateRule()
	explicit.Priority = 5
	explicit.Provider.SystemName = "explicit-priority"
	orch.CreateRule(explicit)

	zero := validCreateRule()
	zero.Priority = 0 // zero value = lowest
	zero.Provider.SystemName = "zero-priority"
	orch.CreateRule(zero)

	resp, _ := orch.Orchestrate(validOrchRequest())
	if len(resp.Response) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "explicit-priority" {
		t.Errorf("priority-5 should come before priority-0, got %q first", resp.Response[0].Provider.SystemName)
	}
	if resp.Response[1].Provider.SystemName != "zero-priority" {
		t.Errorf("priority-0 should be last, got %q", resp.Response[1].Provider.SystemName)
	}
}

func TestOrchestratePriorityInResult(t *testing.T) {
	orch := newOrchestrator()
	req := validCreateRule()
	req.Priority = 7
	orch.CreateRule(req)

	resp, _ := orch.Orchestrate(validOrchRequest())
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result")
	}
	if resp.Response[0].Priority != 7 {
		t.Errorf("Priority in result = %d, want 7", resp.Response[0].Priority)
	}
}

// ---- MetadataFilter matching ----

func TestOrchestrateMetadataFilterMatch(t *testing.T) {
	orch := newOrchestrator()
	req := validCreateRule()
	req.MetadataFilter = map[string]string{"region": "eu"}
	orch.CreateRule(req)

	// Request includes the required metadata key.
	orchReq := validOrchRequest()
	orchReq.RequestedService.Metadata = map[string]string{"region": "eu", "unit": "celsius"}
	resp, _ := orch.Orchestrate(orchReq)
	if len(resp.Response) != 1 {
		t.Errorf("expected 1 result for matching metadata, got %d", len(resp.Response))
	}
}

func TestOrchestrateMetadataFilterNoMatch(t *testing.T) {
	orch := newOrchestrator()
	req := validCreateRule()
	req.MetadataFilter = map[string]string{"region": "eu"}
	orch.CreateRule(req)

	orchReq := validOrchRequest()
	orchReq.RequestedService.Metadata = map[string]string{"region": "us"}
	resp, _ := orch.Orchestrate(orchReq)
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results for mismatched metadata, got %d", len(resp.Response))
	}
}

func TestOrchestrateEmptyFilterMatchesAll(t *testing.T) {
	orch := newOrchestrator()
	// Rule has no metadataFilter — matches any request.
	orch.CreateRule(validCreateRule())

	orchReq := validOrchRequest()
	orchReq.RequestedService.Metadata = map[string]string{"region": "anywhere"}
	resp, _ := orch.Orchestrate(orchReq)
	if len(resp.Response) != 1 {
		t.Errorf("expected 1 result for empty filter, got %d", len(resp.Response))
	}
}

func TestOrchestrateMetadataFilterMissingKey(t *testing.T) {
	orch := newOrchestrator()
	req := validCreateRule()
	req.MetadataFilter = map[string]string{"region": "eu"}
	orch.CreateRule(req)

	// Request has no metadata at all.
	orchReq := validOrchRequest()
	resp, _ := orch.Orchestrate(orchReq)
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results when filter key absent from request, got %d", len(resp.Response))
	}
}

// ---- No match / validation ----

func TestOrchestrateNoMatch(t *testing.T) {
	orch := newOrchestrator()
	orch.CreateRule(validCreateRule())

	resp, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app"},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "other-service"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Response))
	}
}

func TestOrchestrateMissingRequester(t *testing.T) {
	orch := newOrchestrator()
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	})
	if err == nil {
		t.Fatal("expected error for missing requesterSystem.systemName")
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
	if err := newOrchestrator().DeleteRule(999); err == nil {
		t.Fatal("expected error for nonexistent rule")
	}
}
