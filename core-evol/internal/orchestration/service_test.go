package orchestration

import (
	"errors"
	"testing"
)

// --- stubs ---

type stubRegistry struct {
	instances []ServiceInstance
	err       error
}

func (s *stubRegistry) QuerySR(_ ServiceFilter) ([]ServiceInstance, error) {
	return s.instances, s.err
}

type stubDecider struct {
	permit bool
	err    error
	calls  int
}

func (s *stubDecider) Decide(_, _, _, _ string) (bool, error) {
	s.calls++
	return s.permit, s.err
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
	if af.calls != 1 {
		t.Errorf("expected 1 AF call (single decision), got %d", af.calls)
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
}

func TestOrchestrate_AuthDisabled_SkipsXACML(t *testing.T) {
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
	if af.calls != 0 {
		t.Errorf("expected no AF calls when auth disabled, got %d", af.calls)
	}
}

func TestOrchestrate_AFUnavailable_FailClosed(t *testing.T) {
	reg := &stubRegistry{instances: twoProviders()}
	af := &stubDecider{err: errors.New("connection refused")}

	orch := NewXACMLOrchestrator(reg, af, "domain-1", true)
	resp, err := orch.Orchestrate(validReq())

	if err != nil {
		t.Fatalf("expected fail-closed (no error returned), got: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected empty on AF error (fail-closed), got %d", len(resp.Response))
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

func TestOrchestrate_XACMLAttributes_UsesConsumerAndService(t *testing.T) {
	// Verify the XACML decision is made with the correct subject/resource/action.
	type call struct{ domainID, subject, resource, action string }
	var recorded call
	recorder := &recordingDecider{
		fn: func(domainID, subject, resource, action string) (bool, error) {
			recorded = call{domainID, subject, resource, action}
			return true, nil
		},
	}
	reg := &stubRegistry{instances: twoProviders()}

	orch := NewXACMLOrchestrator(reg, recorder, "my-domain", true)
	req := OrchestrationRequest{
		RequesterSystem:  System{SystemName: "my-consumer"},
		RequestedService: ServiceFilter{ServiceDefinition: "telemetry"},
	}
	_, err := orch.Orchestrate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recorded.domainID != "my-domain" {
		t.Errorf("domainID: got %q", recorded.domainID)
	}
	if recorded.subject != "my-consumer" {
		t.Errorf("subject: got %q", recorded.subject)
	}
	if recorded.resource != "telemetry" {
		t.Errorf("resource: got %q", recorded.resource)
	}
	if recorded.action != "consume" {
		t.Errorf("action: got %q want %q", recorded.action, "consume")
	}
}

type recordingDecider struct {
	fn func(domainID, subject, resource, action string) (bool, error)
}

func (r *recordingDecider) Decide(domainID, subject, resource, action string) (bool, error) {
	return r.fn(domainID, subject, resource, action)
}
