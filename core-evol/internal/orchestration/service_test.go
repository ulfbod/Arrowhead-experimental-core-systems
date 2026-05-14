package orchestration

import (
	"errors"
	"testing"
)

// --- stubs ---

type decideCall struct {
	domainID, subject, service, provider, action string
}

type stubDecider struct {
	permit bool
	err    error
	calls  []decideCall
}

func (s *stubDecider) Decide(domainID, subject, service, provider, action string) (bool, error) {
	s.calls = append(s.calls, decideCall{domainID, subject, service, provider, action})
	return s.permit, s.err
}

// funcDecider allows per-call logic for partial-permit tests.
type funcDecider struct {
	fn    func(domainID, subject, service, provider, action string) (bool, error)
	calls []decideCall
}

func (f *funcDecider) Decide(domainID, subject, service, provider, action string) (bool, error) {
	f.calls = append(f.calls, decideCall{domainID, subject, service, provider, action})
	return f.fn(domainID, subject, service, provider, action)
}

type stubRegistry struct {
	instances []ServiceInstance
	err       error
}

func (s *stubRegistry) QuerySR(_ ServiceFilter) ([]ServiceInstance, error) {
	return s.instances, s.err
}

// --- helpers ---

func twoProviders() []ServiceInstance {
	return []ServiceInstance{
		{
			ServiceDefinition: "telemetry",
			Provider:          System{SystemName: "provider-1", Address: "10.0.0.1", Port: 9000},
			ServiceUri:        "/telemetry",
			Interfaces:        []string{"HTTP-SECURE-JSON"},
			Version:           1,
		},
		{
			ServiceDefinition: "telemetry",
			Provider:          System{SystemName: "provider-2", Address: "10.0.0.2", Port: 9000},
			ServiceUri:        "/telemetry",
			Interfaces:        []string{"HTTP-SECURE-JSON"},
			Version:           1,
		},
	}
}

func validReq() OrchestrationRequest {
	return OrchestrationRequest{
		RequesterSystem:  System{SystemName: "consumer-1", Address: "10.0.1.1", Port: 8000},
		RequestedService: ServiceFilter{ServiceDefinition: "telemetry"},
	}
}

// --- tests ---

func TestOrchestrate_Permit_ReturnsAllProviders(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Response))
	}
	// Per-provider: one call per provider
	if len(af.calls) != 2 {
		t.Errorf("expected 2 Decide calls (one per provider), got %d", len(af.calls))
	}
}

func TestOrchestrate_Deny_ReturnsEmpty(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{permit: false}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected empty results on Deny, got %d", len(resp.Response))
	}
	if len(af.calls) != 2 {
		t.Errorf("expected 2 Decide calls (one per provider), got %d", len(af.calls))
	}
}

func TestOrchestrate_PartialPermit_OnlyPermittedProviders(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	// Only provider-1 is permitted.
	af := &funcDecider{fn: func(_, _, _, provider, _ string) (bool, error) {
		return provider == "provider-1", nil
	}}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result (only provider-1), got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "provider-1" {
		t.Errorf("expected provider-1, got %q", resp.Response[0].Provider.SystemName)
	}
	if len(af.calls) != 2 {
		t.Errorf("expected 2 Decide calls, got %d", len(af.calls))
	}
}

func TestOrchestrate_AllProvidersExceptOne(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	// provider-2 is denied; provider-1 permitted.
	af := &funcDecider{fn: func(_, _, _, provider, _ string) (bool, error) {
		return provider != "provider-2", nil
	}}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "provider-1" {
		t.Errorf("wrong provider: got %q", resp.Response[0].Provider.SystemName)
	}
}

func TestOrchestrate_AuthDisabled_SkipsDecider(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{permit: false} // would deny if called

	orch := NewXACMLOrchestrator(reg, af, "domain-1", false)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 2 {
		t.Errorf("expected 2 results when auth disabled, got %d", len(resp.Response))
	}
	if len(af.calls) != 0 {
		t.Errorf("expected no Decide calls when auth disabled, got %d", len(af.calls))
	}
}

func TestOrchestrate_DeciderUnavailable_FailClosed(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{err: errors.New("connection refused")}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("expected fail-closed (no error returned), got: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected empty on decider error (fail-closed per provider), got %d", len(resp.Response))
	}
}

func TestOrchestrate_DeciderUnavailableForOneProvider_OthersPermitted(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	// provider-1 errors; provider-2 permitted
	af := &funcDecider{fn: func(_, _, _, provider, _ string) (bool, error) {
		if provider == "provider-1" {
			return false, errors.New("timeout")
		}
		return true, nil
	}}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Fatalf("expected only provider-2, got %d results", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "provider-2" {
		t.Errorf("wrong provider: got %q", resp.Response[0].Provider.SystemName)
	}
}

func TestOrchestrate_SRUnavailable_ReturnsError(t *testing.T) {
	reg := &stubRegistry{err: errors.New("connection refused")}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	_, err := orch.Orchestrate(validReq())

	if err == nil {
		t.Fatal("expected error from SR failure")
	}
}

func TestOrchestrate_MissingRequester(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	req := validReq()
	req.RequesterSystem.SystemName = ""

	_, err := orch.Orchestrate(req)
	if !errors.Is(err, ErrMissingRequester) {
		t.Errorf("expected ErrMissingRequester, got %v", err)
	}
}

func TestOrchestrate_MissingService(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	req := validReq()
	req.RequestedService.ServiceDefinition = ""

	_, err := orch.Orchestrate(req)
	if !errors.Is(err, ErrMissingService) {
		t.Errorf("expected ErrMissingService, got %v", err)
	}
}

func TestOrchestrate_Permit_ResultFieldsCorrect(t *testing.T) {
	reg := &stubRegistry{instances: []ServiceInstance{{
		ServiceDefinition: "telemetry",
		Provider:          System{SystemName: "prov", Address: "1.2.3.4", Port: 9001},
		ServiceUri:        "/data",
		Interfaces:        []string{"HTTP-SECURE-JSON"},
		Version:           2,
		Metadata:          map[string]string{"zone": "A"},
	}}}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result")
	}
	r := resp.Response[0]
	if r.Provider.SystemName != "prov" {
		t.Errorf("provider name: got %q", r.Provider.SystemName)
	}
	if r.Service.ServiceUri != "/data" {
		t.Errorf("serviceUri: got %q", r.Service.ServiceUri)
	}
	if r.Service.Version != 2 {
		t.Errorf("version: got %d", r.Service.Version)
	}
	if r.Service.Metadata["zone"] != "A" {
		t.Errorf("metadata: got %v", r.Service.Metadata)
	}
}

// TestOrchestrate_DecideFields_SeparateServiceAndProvider verifies that Decide
// is called with service and provider as separate arguments — not concatenated
// as "service@provider". This is the key semantic change from the previous
// resource-encoding approach.
func TestOrchestrate_DecideFields_SeparateServiceAndProvider(t *testing.T) {
	reg := &stubRegistry{instances: []ServiceInstance{{
		ServiceDefinition: "telemetry",
		Provider:          System{SystemName: "robot-fleet-site-1"},
	}}}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "my-domain", true)
	req := OrchestrationRequest{
		RequesterSystem:  System{SystemName: "portal-cloud-ml"},
		RequestedService: ServiceFilter{ServiceDefinition: "telemetry"},
	}
	_, err := orch.Orchestrate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(af.calls) != 1 {
		t.Fatalf("expected 1 Decide call, got %d", len(af.calls))
	}
	c := af.calls[0]
	if c.domainID != "my-domain" {
		t.Errorf("domainID: got %q", c.domainID)
	}
	if c.subject != "portal-cloud-ml" {
		t.Errorf("subject: got %q", c.subject)
	}
	// service and provider are separate — NOT "telemetry@robot-fleet-site-1"
	if c.service != "telemetry" {
		t.Errorf("service: got %q, want %q", c.service, "telemetry")
	}
	if c.provider != "robot-fleet-site-1" {
		t.Errorf("provider: got %q, want %q", c.provider, "robot-fleet-site-1")
	}
	if c.action != "orchestrate" {
		t.Errorf("action: got %q, want %q", c.action, "orchestrate")
	}
}

func TestOrchestrate_EmptySR_ReturnsEmpty(t *testing.T) {
	reg := &stubRegistry{instances: []ServiceInstance{}}
	af := &stubDecider{permit: true}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected empty, got %d", len(resp.Response))
	}
	if len(af.calls) != 0 {
		t.Errorf("expected no Decide calls for empty SR, got %d", len(af.calls))
	}
}
