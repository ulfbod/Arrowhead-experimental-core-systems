package model_test

import (
	"encoding/json"
	"testing"

	"arrowhead/core/internal/orchestration/model"
)

// ─── Cycle 17.1 — Dual-decode: serviceRequirement and requestedService ─────────

func TestOrchestrationRequestDecodesServiceRequirement(t *testing.T) {
	raw := `{"requesterSystem":{"systemName":"C"},"serviceRequirement":{"serviceDefinition":"temp"}}`
	var req model.OrchestrationRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.RequestedService.ServiceDefinition != "temp" {
		t.Errorf("ServiceDefinition = %q, want temp", req.RequestedService.ServiceDefinition)
	}
}

func TestOrchestrationRequestDecodesRequestedServiceBackwardCompat(t *testing.T) {
	raw := `{"requesterSystem":{"systemName":"C"},"requestedService":{"serviceDefinition":"temp"}}`
	var req model.OrchestrationRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.RequestedService.ServiceDefinition != "temp" {
		t.Errorf("ServiceDefinition = %q, want temp", req.RequestedService.ServiceDefinition)
	}
}

func TestOrchestrationRequestMarshalUsesServiceRequirement(t *testing.T) {
	req := model.OrchestrationRequest{
		RequesterSystem:  model.System{SystemName: "C"},
		RequestedService: model.ServiceRequirement{ServiceDefinition: "temp"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["serviceRequirement"]; !ok {
		t.Error("encoded output missing serviceRequirement key")
	}
	if _, ok := raw["requestedService"]; ok {
		t.Error("encoded output must not contain requestedService key")
	}
}

// ─── Cycle 17.2 — Response field names include spec typos ────────────────────

func TestOrchestrationResultSpecTypoFieldNames(t *testing.T) {
	result := model.OrchestrationResult{
		ServiceDefinition: "temp",
		ProviderName:      "sensor-1",
	}
	data, _ := json.Marshal(result)
	var raw map[string]any
	json.Unmarshal(data, &raw)

	// Spec typo: double 't' in serviceDefinitition
	if _, ok := raw["serviceDefinitition"]; !ok {
		t.Errorf("JSON key serviceDefinitition missing (double t) — got keys: %v", keys(raw))
	}
	// Spec typo: missing 'n' in cloudIdentitifer
	if _, ok := raw["cloudIdentitifer"]; ok {
		// cloudIdentitifer is omitempty — only present when non-empty
		// This test verifies the field NAME is correct when populated.
	}
	// Old key must NOT be present.
	if _, ok := raw["serviceDefinition"]; ok {
		t.Error("old key serviceDefinition must not appear in encoded output")
	}
}

func TestOrchestrationResultCloudIdentifierTypo(t *testing.T) {
	result := model.OrchestrationResult{
		ServiceDefinition: "temp",
		ProviderName:      "sensor-1",
		CloudIdentifier:   "LOCAL",
	}
	data, _ := json.Marshal(result)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	// Spec typo: missing 'n' in cloudIdentitifer (not cloudIdentifier)
	if _, ok := raw["cloudIdentitifer"]; !ok {
		t.Errorf("JSON key cloudIdentitifer missing (missing n) — got keys: %v", keys(raw))
	}
	if _, ok := raw["cloudIdentifier"]; ok {
		t.Error("old key cloudIdentifier must not appear in encoded output")
	}
}

func TestOrchestrationResponseUsesResultsField(t *testing.T) {
	resp := model.OrchestrationResponse{
		Results: []model.OrchestrationResult{{ProviderName: "p1"}},
	}
	data, _ := json.Marshal(resp)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["results"]; !ok {
		t.Error("encoded output missing results key")
	}
	if _, ok := raw["response"]; ok {
		t.Error("old key response must not appear in encoded output")
	}
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
