package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	blclient "arrowhead/core/internal/blacklist/client"
	dynclient "arrowhead/core/internal/orchestration/dynamic/client"
	"arrowhead/core/internal/orchestration/dynamic/service"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

// srResponse builds a fake AH5 ServiceRegistry service-discovery/lookup response.
func srResponse(providers ...string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		type provider struct {
			Name string `json:"name"`
		}
		type iface struct {
			TemplateName string `json:"templateName"`
		}
		type instance struct {
			InstanceID            string   `json:"instanceId"`
			Provider              provider `json:"provider"`
			ServiceDefinitionName string   `json:"serviceDefinitionName"`
			Interfaces            []iface  `json:"interfaces"`
			Version               string   `json:"version"`
		}
		type response struct {
			Entries []instance `json:"entries"`
			Count   int        `json:"count"`
		}
		var instances []instance
		for i, p := range providers {
			instances = append(instances, instance{
				InstanceID:            p + "|temperature-service|1",
				Provider:              provider{Name: p},
				ServiceDefinitionName: "temperature-service",
				Interfaces:            []iface{{TemplateName: "HTTP"}},
				Version:               fmt.Sprintf("%d", i+1),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{Entries: instances, Count: len(instances)})
	}
}

// caResponse builds a fake ConsumerAuthorization verify response (plain JSON Boolean).
func caAlwaysAuthorized() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(true)
	}
}

func caAlwaysDenied() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(false)
	}
}

// fakeAuthSys builds a fake Authentication identity/verify response.
func fakeAuthSys(valid bool, systemName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"verified": valid, "systemName": systemName})
	}
}

// newOrchestrator creates a DynamicOrchestrator with no identity check.
func newOrchestrator(srURL, caURL string, checkAuth bool) *service.DynamicOrchestrator {
	return service.NewDynamicOrchestrator(srURL, caURL, "", checkAuth, false)
}

func validRequest() orchmodel.OrchestrationRequest {
	return orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app", Address: "localhost", Port: 0},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	}
}

// ---- Without auth ----

func TestOrchestrateDynamicNoAuth(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results (auth disabled), got %d", len(resp.Results))
	}
}

func TestOrchestrateDynamicEmptySR(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse()))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results for empty SR, got %d", len(resp.Results))
	}
}

// ---- With auth ----

func TestOrchestrateDynamicAuthAllAllowed(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results (all authorized), got %d", len(resp.Results))
	}
}

func TestOrchestrateDynamicAuthAllDenied(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysDenied()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results (all denied), got %d", len(resp.Results))
	}
}

func TestOrchestrateDynamicAuthPartial(t *testing.T) {
	// CA authorizes only "sensor-1", denies "sensor-2".
	allowedProvider := "sensor-1"
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string `json:"provider"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.Provider == allowedProvider
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authorized)
	}))
	defer ca.Close()

	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 authorized result, got %d", len(resp.Results))
	}
	if resp.Results[0].ProviderName != "sensor-1" {
		t.Errorf("expected sensor-1, got %q", resp.Results[0].ProviderName)
	}
}

func TestOrchestrateDynamicCAUnreachableFailsClosed(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// Point CA at a closed port to simulate unreachable service.
	orch := newOrchestrator(sr.URL, "http://127.0.0.1:1", true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fail-closed: unreachable CA means provider is excluded.
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results when CA unreachable (fail-closed), got %d", len(resp.Results))
	}
}

func TestOrchestrateDynamicSRUnreachable(t *testing.T) {
	orch := newOrchestrator("http://127.0.0.1:1", "http://127.0.0.1:1", false)
	_, err := orch.Orchestrate(validRequest(), "")
	if err == nil {
		t.Error("expected error when SR is unreachable")
	}
}

// ---- Validation ----

func TestOrchestrateDynamicMissingRequester(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse()))
	defer sr.Close()
	orch := newOrchestrator(sr.URL, "", false)
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "svc"},
	}, "")
	if err == nil {
		t.Fatal("expected error for missing requesterSystem.systemName")
	}
}

func TestOrchestrateDynamicMissingService(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse()))
	defer sr.Close()
	orch := newOrchestrator(sr.URL, "", false)
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem: orchmodel.System{SystemName: "consumer"},
	}, "")
	if err == nil {
		t.Fatal("expected error for missing serviceDefinition")
	}
}

// ---- Identity check ----

func TestOrchestrateIdentityRequired(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(true, "consumer-app")))
	defer authSys.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", authSys.URL, false, true)
	_, err := orch.Orchestrate(validRequest(), "") // empty token
	if err != service.ErrIdentityRequired {
		t.Errorf("expected ErrIdentityRequired, got %v", err)
	}
}

func TestOrchestrateIdentityInvalidToken(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(false, "")))
	defer authSys.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", authSys.URL, false, true)
	_, err := orch.Orchestrate(validRequest(), "bad-token")
	if err != service.ErrIdentityInvalid {
		t.Errorf("expected ErrIdentityInvalid, got %v", err)
	}
}

func TestOrchestrateIdentityAuthSysUnreachable(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// Auth system unreachable — fail-closed.
	orch := service.NewDynamicOrchestrator(sr.URL, "", "http://127.0.0.1:1", false, true)
	_, err := orch.Orchestrate(validRequest(), "some-token")
	if err == nil {
		t.Error("expected error when auth system unreachable (fail-closed)")
	}
}

func TestOrchestrateIdentityVerifiedNameUsedForCACheck(t *testing.T) {
	// The requester sends systemName "impersonator" in the body, but the
	// verified token belongs to "consumer-app". The CA only authorizes
	// "consumer-app" — if the orchestrator correctly uses the verified name,
	// the result should be non-empty; if it uses the self-reported name, it would be empty.
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(true, "consumer-app")))
	defer authSys.Close()

	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Consumer string `json:"consumer"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.Consumer == "consumer-app"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authorized)
	}))
	defer ca.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, ca.URL, authSys.URL, true, true)

	// Self-reported name is "impersonator" — should be overridden to "consumer-app".
	req := orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "impersonator", Address: "localhost", Port: 0},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	}
	resp, err := orch.Orchestrate(req, "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result (verified name used for CA), got %d", len(resp.Results))
	}
}

func TestOrchestrateIdentityDisabledNoTokenNeeded(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// checkIdentity=false — no token needed, should work fine.
	orch := service.NewDynamicOrchestrator(sr.URL, "", "http://127.0.0.1:1", false, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result when identity check disabled, got %d", len(resp.Results))
	}
}

// ---- Malformed upstream responses ----

func TestOrchestrateSRMalformedJSON(t *testing.T) {
	// SR returns malformed JSON — querySR decode error should propagate as SR error.
	sr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	_, err := orch.Orchestrate(validRequest(), "")
	if err == nil {
		t.Error("expected error when SR returns malformed JSON")
	}
}

func TestOrchestrateIdentityAuthSysMalformedJSON(t *testing.T) {
	// Auth system returns malformed JSON — decode error → fail-closed.
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	authSys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer authSys.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", authSys.URL, false, true)
	_, err := orch.Orchestrate(validRequest(), "some-token")
	if err != service.ErrIdentityInvalid {
		t.Errorf("expected ErrIdentityInvalid when auth sys returns malformed JSON, got %v", err)
	}
}

func TestOrchestrateIdentityInvalidAuthURL(t *testing.T) {
	// An authSysURL containing a control character causes http.NewRequest to fail.
	// The orchestrator must still return ErrIdentityInvalid (fail-closed).
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", "http://invalid\nurl", false, true)
	_, err := orch.Orchestrate(validRequest(), "some-token")
	if err != service.ErrIdentityInvalid {
		t.Errorf("expected ErrIdentityInvalid for malformed authSysURL, got %v", err)
	}
}

func TestOrchestrateCAMalformedJSONExcludesProvider(t *testing.T) {
	// CA returns malformed JSON — checkAuthorized decode error should exclude provider (fail-closed, D4).
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fail-closed: malformed CA response means provider is excluded.
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results when CA returns malformed JSON (fail-closed), got %d", len(resp.Results))
	}
}

// ─── Cycle 17.3 — DynamicOrch calls AH5 lookup endpoint ─────────────────────

func TestDynamicOrchCallsAH5LookupEndpoint(t *testing.T) {
	called := false
	fakeSR := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/serviceregistry/service-discovery/lookup" {
			called = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"entries": []any{}, "count": 0})
	}))
	defer fakeSR.Close()

	orch := service.NewDynamicOrchestrator(fakeSR.URL, "", "", false, false)
	orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "C"},
		RequestedService: orchmodel.ServiceRequirement{ServiceDefinition: "temp"},
	}, "")
	if !called {
		t.Error("DynamicOrchestrator did not call AH5 lookup endpoint")
	}
}

// ─── Step B: Tests for Step 24 ────────────────────────────────────────────────

func TestOrchestrationResultHasCloudIdentifier(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].CloudIdentifier != "LOCAL" {
		t.Errorf("CloudIdentifier = %q, want \"LOCAL\"", resp.Results[0].CloudIdentifier)
	}
}

func TestOrchestrationResultNoExclusiveUntilWhenUnlocked(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].ExclusiveUntil != "" {
		t.Errorf("ExclusiveUntil = %q, want empty string (no lock)", resp.Results[0].ExclusiveUntil)
	}
}

func TestOrchestrationResultInterfacesForwarded(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if len(resp.Results[0].Interfaces) == 0 {
		t.Errorf("Interfaces is empty, expected SR interfaces to be forwarded")
	}
}

// ─── Step 28 (G42): Blacklist integration — Dynamic Orchestrator ──────────────

// newOrchestratorWithBlacklist wires up a DynamicOrchestrator with a given blacklist server.
func newOrchestratorWithBlacklist(srURL, caURL string, blURL string) *service.DynamicOrchestrator {
	var bl blclient.BlacklistClient
	if blURL != "" {
		bl = blclient.NewHTTPClient(blURL, http.DefaultClient)
	} else {
		bl = blclient.NopClient{}
	}
	return service.NewDynamicOrchestratorWithClients(
		dynclient.NewSRHTTPClient(srURL, http.DefaultClient),
		dynclient.NewCAHTTPClient(caURL, http.DefaultClient),
		nil,
		bl,
		false, false,
	)
}

func TestBlacklistedProviderExcludedFromResults(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("blacklisted-sensor", "good-sensor")))
	defer sr.Close()

	// Blacklist returns true only for "blacklisted-sensor".
	blacklist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len("/blacklist/check/"):]
		json.NewEncoder(w).Encode(name == "blacklisted-sensor") //nolint:errcheck
	}))
	defer blacklist.Close()

	orch := newOrchestratorWithBlacklist(sr.URL, "", blacklist.URL)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result after blacklist filter, got %d", len(resp.Results))
	}
	if resp.Results[0].ProviderName == "blacklisted-sensor" {
		t.Error("blacklisted provider should have been excluded")
	}
}

func TestBlacklistedRequesterRejected(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// Blacklist returns true for all systems (to catch the requester check).
	blacklist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(true) //nolint:errcheck
	}))
	defer blacklist.Close()

	orch := newOrchestratorWithBlacklist(sr.URL, "", blacklist.URL)
	_, err := orch.Orchestrate(validRequest(), "")
	if err == nil {
		t.Error("expected error for blacklisted requester, got nil")
	}
}

// ─── Step 31: Push notification delivery ──────────────────────────────────────

func newOrch(srURL, caURL string) *service.DynamicOrchestrator {
	return service.NewDynamicOrchestratorWithClients(
		dynclient.NewSRHTTPClient(srURL, http.DefaultClient),
		dynclient.NewCAHTTPClient(caURL, http.DefaultClient),
		nil,
		blclient.NopClient{},
		false, false,
	)
}

func makeSub(notifyURL string) service.Subscription {
	return service.Subscription{
		ID:              "sub-test-1",
		OwnerSystemName: "consumer-1",
		TargetSystemName: "svc-def",
		NotifyInterface: map[string]any{"notifyUri": notifyURL},
	}
}

func waitForHistory(t *testing.T, orch *service.DynamicOrchestrator, wantStatus string, maxWaitMs int) service.HistoryEntry {
	t.Helper()
	deadline := time.Now().Add(time.Duration(maxWaitMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		hist := orch.QueryHistory()
		for _, e := range hist.Entries {
			if e.Status == wantStatus {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for history entry with status=%q; entries: %+v", wantStatus, orch.QueryHistory().Entries)
	return service.HistoryEntry{}
}

func TestPushTriggerDeliversToPushSubscriber(t *testing.T) {
	var received []byte
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received) //nolint:errcheck
		w.WriteHeader(http.StatusOK)
	}))
	defer subscriber.Close()

	orch := newOrch("", "")
	orch.SetPushClient(&http.Client{Timeout: 2 * time.Second})

	sub := makeSub(subscriber.URL)
	orch.TriggerPush(sub)

	// Poll for DELIVERED status (goroutine delivery is async).
	entry := waitForHistory(t, orch, "DELIVERED", 2000)
	if entry.Status != "DELIVERED" {
		t.Errorf("want DELIVERED, got %q", entry.Status)
	}
}

func TestPushDeliveryFailureRecordedInHistory(t *testing.T) {
	// Subscriber at unreachable address.
	orch := newOrch("", "")
	orch.SetPushClient(&http.Client{Timeout: 100 * time.Millisecond})

	sub := makeSub("http://127.0.0.1:19999/no-such-endpoint")
	orch.TriggerPush(sub)

	entry := waitForHistory(t, orch, "FAILED", 1000)
	if entry.Status != "FAILED" {
		t.Errorf("want FAILED, got %q", entry.Status)
	}
}

func TestPushDeliveryTimeoutRespected(t *testing.T) {
	// Subscriber that hangs until the client closes.
	done := make(chan struct{})
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-done // never responds
	}))
	defer subscriber.Close()
	defer close(done)

	orch := newOrch("", "")
	orch.SetPushClient(&http.Client{Timeout: 100 * time.Millisecond})

	sub := makeSub(subscriber.URL)
	orch.TriggerPush(sub)

	entry := waitForHistory(t, orch, "FAILED", 1000)
	if entry.Status != "FAILED" {
		t.Errorf("want FAILED after timeout, got %q", entry.Status)
	}
}

func TestPushTriggerDoesNotBlockHandler(t *testing.T) {
	// Subscriber that is slow to respond.
	slow := make(chan struct{})
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-slow
	}))
	defer subscriber.Close()

	orch := newOrch("", "")
	orch.SetPushClient(&http.Client{Timeout: 5 * time.Second})

	// TriggerPush must return before the subscriber responds.
	done := make(chan struct{})
	go func() {
		orch.TriggerPush(makeSub(subscriber.URL))
		close(done)
	}()
	select {
	case <-done:
		// OK — TriggerPush returned immediately.
	case <-time.After(100 * time.Millisecond):
		t.Error("TriggerPush blocked longer than 100ms")
	}
	close(slow)
}

func TestMissingNotifyURLRecordsFailure(t *testing.T) {
	orch := newOrch("", "")
	sub := service.Subscription{
		ID:              "sub-no-url",
		OwnerSystemName: "consumer",
		NotifyInterface: map[string]any{}, // no URL fields
	}
	orch.TriggerPush(sub)
	entry := waitForHistory(t, orch, "FAILED", 500)
	if entry.Status != "FAILED" {
		t.Errorf("want FAILED for missing notify URL, got %q", entry.Status)
	}
}

// ─── Step 36 (G40): QoS filtering ───────────────────────────────────────────

// mockQoSClient implements dynclient.QoSEvaluatorClient for testing.
type mockQoSClient struct {
	latency   int64
	reachable bool
	err       error
}

func (m *mockQoSClient) Measure(_ context.Context, _, _ string) (int64, bool, error) {
	return m.latency, m.reachable, m.err
}

func TestQoSFilterPassesFastProvider(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("fast-provider")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	orch.SetQoSClient(&mockQoSClient{latency: 10, reachable: true})

	req := validRequest()
	req.QualityRequirements = []orchmodel.QoSRequirement{{MaxLatencyMs: 100}}
	resp, err := orch.Orchestrate(req, "")
	if err != nil {
		t.Fatalf("orchestrate: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result (fast provider passes), got %d", len(resp.Results))
	}
}

func TestQoSFilterNoRequirementsPassesAll(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("provider-1", "provider-2")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	orch.SetQoSClient(&mockQoSClient{latency: 9999, reachable: true})

	req := validRequest()
	// No qualityRequirements → QoS client is not consulted
	resp, err := orch.Orchestrate(req, "")
	if err != nil {
		t.Fatalf("orchestrate: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Results))
	}
}

func TestQoSEvaluatorUnreachablePassesCandidate(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("provider-1")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	// QoS evaluator returns error → fail-open → include candidate
	orch.SetQoSClient(&mockQoSClient{err: fmt.Errorf("qos evaluator unavailable")})

	req := validRequest()
	req.QualityRequirements = []orchmodel.QoSRequirement{{MaxLatencyMs: 50}}
	resp, err := orch.Orchestrate(req, "")
	if err != nil {
		t.Fatalf("orchestrate: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Errorf("fail-open: expected 1 result, got %d", len(resp.Results))
	}
}

func TestQoSFilterExcludesUnreachableProvider(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("slow-provider")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	// Provider is not reachable
	orch.SetQoSClient(&mockQoSClient{latency: 0, reachable: false})

	req := validRequest()
	req.QualityRequirements = []orchmodel.QoSRequirement{{MaxLatencyMs: 100}}
	resp, err := orch.Orchestrate(req, "")
	if err != nil {
		t.Fatalf("orchestrate: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("unreachable provider should be excluded, got %d results", len(resp.Results))
	}
}

// ---- Translation (G36) ----

// mockTranslationClient records whether CanTranslate was called and returns a fixed answer.
type mockTranslationClient struct {
	called   bool
	canXlate bool
}

func (m *mockTranslationClient) CanTranslate(_ context.Context, _, _ string) (bool, error) {
	m.called = true
	return m.canXlate, nil
}

func TestOrchestrationWithAllowTranslationUsesTranslationMgr(t *testing.T) {
	// SR returns a provider with interface "HTTP" (not the requested "MQTT-JSON").
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)

	mock := &mockTranslationClient{canXlate: true}
	orch.SetTranslationClient(mock)

	req := validRequest()
	req.OrchestrationFlags.AllowTranslation = true
	// The fake SR returns providers with interface "HTTP"; request asks for "MQTT-JSON".
	req.RequestedService.Interfaces = []string{"MQTT-JSON"}

	resp, err := orch.Orchestrate(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected TranslationClient.CanTranslate to be called when ALLOW_TRANSLATION=true and interface mismatch")
	}
	// TranslationClient returned true → provider included despite interface mismatch.
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result (translation allowed), got %d", len(resp.Results))
	}
}

func TestOrchestrationWithoutTranslationClientFiltersInterfaceMismatch(t *testing.T) {
	// SR returns provider with interface "HTTP"; request asks for "MQTT-JSON" + ALLOW_TRANSLATION=true
	// but no TranslationClient is set → flag is a no-op, providers are returned as-is.
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	// No SetTranslationClient call → translationClient is nil.

	req := validRequest()
	req.OrchestrationFlags.AllowTranslation = true
	req.RequestedService.Interfaces = []string{"MQTT-JSON"}

	resp, err := orch.Orchestrate(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No translationClient wired → interface filtering does not apply → providers pass through.
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result (no filter without translationClient), got %d", len(resp.Results))
	}
}
