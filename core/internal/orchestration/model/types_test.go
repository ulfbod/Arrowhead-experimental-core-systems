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

// ─── Cycle 17.2 — Response field names match official AH5 docs (June 2026) ───

func TestOrchestrationResultSpecTypoFieldNames(t *testing.T) {
	result := model.OrchestrationResult{
		ServiceDefinition: "temp",
		ProviderName:      "sensor-1",
	}
	data, _ := json.Marshal(result)
	var raw map[string]any
	json.Unmarshal(data, &raw)

	// Correct key: serviceDefinition (single 't')
	if _, ok := raw["serviceDefinition"]; !ok {
		t.Errorf("JSON key serviceDefinition missing — got keys: %v", keys(raw))
	}
	// Old typo key must NOT be present.
	if _, ok := raw["serviceDefinitition"]; ok {
		t.Error("typo key 'serviceDefinitition' must not appear in JSON output")
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
	// Correct key: cloudIdentifier (with 'n')
	if v, ok := raw["cloudIdentifier"]; !ok || v != "LOCAL" {
		t.Errorf("expected JSON key 'cloudIdentifier'='LOCAL', got %v", raw)
	}
	if _, ok := raw["cloudIdentitifer"]; ok {
		t.Error("typo key 'cloudIdentitifer' must not appear in JSON output")
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

// ─── Cycle 59 — versionRequirement in ServiceRequirement (G55) ───────────────

func TestOrchestrationRequestVersionRequirementRoundTrips(t *testing.T) {
	req := model.OrchestrationRequest{
		RequesterSystem:  model.System{SystemName: "C"},
		RequestedService: model.ServiceRequirement{ServiceDefinition: "temp", VersionRequirement: "2.0.0"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	sr, ok := raw["serviceRequirement"].(map[string]any)
	if !ok {
		t.Fatal("serviceRequirement not a map")
	}
	if v, ok := sr["versionRequirement"]; !ok || v != "2.0.0" {
		t.Errorf("versionRequirement missing or wrong: %v", sr)
	}

	// Decode it back.
	var req2 model.OrchestrationRequest
	if err := json.Unmarshal(data, &req2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req2.RequestedService.VersionRequirement != "2.0.0" {
		t.Errorf("decoded VersionRequirement = %q, want 2.0.0", req2.RequestedService.VersionRequirement)
	}
}

func TestOrchestrationRequestVersionRequirementOmittedWhenEmpty(t *testing.T) {
	req := model.OrchestrationRequest{
		RequesterSystem:  model.System{SystemName: "C"},
		RequestedService: model.ServiceRequirement{ServiceDefinition: "temp"},
	}
	data, _ := json.Marshal(req)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	sr := raw["serviceRequirement"].(map[string]any)
	if _, ok := sr["versionRequirement"]; ok {
		t.Error("versionRequirement should be omitted when empty")
	}
}

// ─── Cycle 65 — G59+G61: correct JSON field names in OrchestrationResult ────

func TestOrchestrationResultUsesCorrectServiceDefinitionKey(t *testing.T) {
	r := model.OrchestrationResult{ServiceDefinition: "temperature"}
	data, _ := json.Marshal(r)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["serviceDefinition"]; !ok {
		t.Errorf("expected JSON key 'serviceDefinition', got keys: %v", keys(raw))
	}
	if _, ok := raw["serviceDefinitition"]; ok {
		t.Error("typo key 'serviceDefinitition' must not appear in JSON output")
	}
}

func TestOrchestrationResultUsesCorrectCloudIdentifierKey(t *testing.T) {
	r := model.OrchestrationResult{CloudIdentifier: "LOCAL"}
	data, _ := json.Marshal(r)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if v, ok := raw["cloudIdentifier"]; !ok || v != "LOCAL" {
		t.Errorf("expected JSON key 'cloudIdentifier'='LOCAL', got %v", raw)
	}
	if _, ok := raw["cloudIdentitifer"]; ok {
		t.Error("typo key 'cloudIdentitifer' must not appear in JSON output")
	}
}

func TestOrchestrationResultUsesAuthorizationTokenSingular(t *testing.T) {
	desc := &model.AuthorizationTokenDescriptor{TokenType: "TIME_LIMITED_TOKEN", Token: "t1"}
	r := model.OrchestrationResult{
		AuthorizationToken: map[string]map[string]*model.AuthorizationTokenDescriptor{
			"HTTP-INSECURE-JSON": {"": desc},
		},
	}
	data, _ := json.Marshal(r)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["authorizationToken"]; !ok {
		t.Errorf("expected singular 'authorizationToken', got keys: %v", raw)
	}
	if _, ok := raw["authorizationTokens"]; ok {
		t.Error("plural 'authorizationTokens' must not appear")
	}
}
