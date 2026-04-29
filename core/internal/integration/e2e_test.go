// Package integration provides end-to-end tests that wire real system handlers
// together in-process using httptest.Server. No mocking of business logic occurs;
// each system runs its actual service and repository layers.
package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	srapi "arrowhead/core/internal/api"
	srrepo "arrowhead/core/internal/repository"
	srsvc "arrowhead/core/internal/service"

	authapi "arrowhead/core/internal/authentication/api"
	authrepo "arrowhead/core/internal/authentication/repository"
	authsvc "arrowhead/core/internal/authentication/service"

	caapi "arrowhead/core/internal/consumerauth/api"
	carepo "arrowhead/core/internal/consumerauth/repository"
	casvc "arrowhead/core/internal/consumerauth/service"

	dynapi "arrowhead/core/internal/orchestration/dynamic/api"
	dynsvc "arrowhead/core/internal/orchestration/dynamic/service"

	ssapi "arrowhead/core/internal/orchestration/simplestore/api"
	ssrepo "arrowhead/core/internal/orchestration/simplestore/repository"
	sssvc "arrowhead/core/internal/orchestration/simplestore/service"

	fsapi "arrowhead/core/internal/orchestration/flexiblestore/api"
	fsrepo "arrowhead/core/internal/orchestration/flexiblestore/repository"
	fssvc "arrowhead/core/internal/orchestration/flexiblestore/service"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func post(t *testing.T, server *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := server.Client().Post(server.URL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func mustPost(t *testing.T, server *httptest.Server, path string, body any, wantStatus int) map[string]any {
	t.Helper()
	resp := post(t, server, path, body)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s: expected %d, got %d", path, wantStatus, resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func postWithToken(t *testing.T, server *httptest.Server, path string, body any, token string) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func deleteAt(t *testing.T, server *httptest.Server, path string, wantStatus int) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, server.URL+path, nil)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("DELETE %s: expected %d, got %d", path, wantStatus, resp.StatusCode)
	}
}

func deleteJSON(t *testing.T, server *httptest.Server, path string, body string, wantStatus int) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, server.URL+path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("DELETE %s: expected %d, got %d", path, wantStatus, resp.StatusCode)
	}
}

func decodeOrchResponse(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var result struct {
		Response []map[string]any `json:"response"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Response
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// ── System factories ──────────────────────────────────────────────────────────

func startSR(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(srapi.NewHandler(srsvc.NewRegistryService(srrepo.NewMemoryRepository())))
}

func startAuth(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(authapi.NewHandler(authsvc.NewAuthService(authrepo.NewMemoryRepository(), time.Hour)))
}

func startCA(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(caapi.NewHandler(casvc.NewAuthService(carepo.NewMemoryRepository())))
}

func startDynOrch(t *testing.T, srURL, caURL, authSysURL string, checkAuth, checkIdentity bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(dynapi.NewHandler(dynsvc.NewDynamicOrchestrator(srURL, caURL, authSysURL, checkAuth, checkIdentity)))
}

func startSimpleStore(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(ssapi.NewHandler(sssvc.NewSimpleStoreOrchestrator(ssrepo.NewMemoryRepository())))
}

func startFlexibleStore(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(fsapi.NewHandler(fssvc.NewFlexibleStoreOrchestrator(fsrepo.NewMemoryRepository())))
}

// ── Common operations ─────────────────────────────────────────────────────────

func registerService(t *testing.T, sr *httptest.Server, systemName, serviceDef string) {
	t.Helper()
	mustPost(t, sr, "/serviceregistry/register", map[string]any{
		"serviceDefinition": serviceDef,
		"providerSystem":    map[string]any{"systemName": systemName, "address": "10.0.0.1", "port": 9000},
		"serviceUri":        "/" + serviceDef,
		"interfaces":        []string{"HTTP-INSECURE-JSON"},
		"version":           1,
	}, http.StatusCreated)
}

func grantRule(t *testing.T, ca *httptest.Server, consumer, provider, service string) {
	t.Helper()
	mustPost(t, ca, "/authorization/grant", map[string]string{
		"consumerSystemName": consumer,
		"providerSystemName": provider,
		"serviceDefinition":  service,
	}, http.StatusCreated)
}

// login calls the Authentication system and returns the issued token.
func login(t *testing.T, auth *httptest.Server, systemName string) string {
	t.Helper()
	result := mustPost(t, auth, "/authentication/identity/login", map[string]string{
		"systemName":  systemName,
		"credentials": "ignored",
	}, http.StatusCreated)
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login: no token returned for %s", systemName)
	}
	return token
}

// ── Dynamic Orchestration ─────────────────────────────────────────────────────

func TestE2EDynamicOrchestrationWithoutAuth(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	registerService(t, sr, "sensor-1", "temperature-service")

	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", false, false)
	defer dynOrch.Close()

	results := decodeOrchResponse(t, post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["provider"].(map[string]any)["systemName"] != "sensor-1" {
		t.Errorf("unexpected provider: %v", results[0]["provider"])
	}
}

func TestE2EDynamicOrchestrationWithAuth(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()

	registerService(t, sr, "sensor-1", "temperature-service")
	registerService(t, sr, "sensor-2", "temperature-service")
	grantRule(t, ca, "consumer-app", "sensor-1", "temperature-service")
	// sensor-2 has no grant.

	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", true, false)
	defer dynOrch.Close()

	results := decodeOrchResponse(t, post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 authorized result, got %d", len(results))
	}
	if results[0]["provider"].(map[string]any)["systemName"] != "sensor-1" {
		t.Errorf("expected sensor-1 (authorized), got %v", results[0]["provider"])
	}
}

func TestE2EDynamicOrchestrationNoGrantNoResults(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	registerService(t, sr, "sensor-1", "temperature-service")
	// No auth rules granted.

	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", true, false)
	defer dynOrch.Close()

	results := decodeOrchResponse(t, post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 0 {
		t.Errorf("expected 0 results (no auth rules), got %d", len(results))
	}
}

// ── SimpleStore Orchestration ─────────────────────────────────────────────────

func TestE2ESimpleStoreOrchestration(t *testing.T) {
	ss := startSimpleStore(t); defer ss.Close()

	mustPost(t, ss, "/orchestration/simplestore/rules", map[string]any{
		"consumerSystemName": "consumer-app",
		"serviceDefinition":  "temperature-service",
		"provider":           map[string]any{"systemName": "sensor-1", "address": "10.0.0.1", "port": 9000},
		"serviceUri":         "/temperature",
		"interfaces":         []string{"HTTP-INSECURE-JSON"},
	}, http.StatusCreated)

	results := decodeOrchResponse(t, post(t, ss, "/orchestration/simplestore", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["provider"].(map[string]any)["systemName"] != "sensor-1" {
		t.Errorf("unexpected provider: %v", results[0]["provider"])
	}
}

func TestE2ESimpleStoreRuleLifecycle(t *testing.T) {
	ss := startSimpleStore(t); defer ss.Close()

	result := mustPost(t, ss, "/orchestration/simplestore/rules", map[string]any{
		"consumerSystemName": "consumer-app",
		"serviceDefinition":  "temperature-service",
		"provider":           map[string]any{"systemName": "sensor-1", "address": "10.0.0.1", "port": 9000},
		"serviceUri":         "/temperature",
		"interfaces":         []string{"HTTP"},
	}, http.StatusCreated)
	id := int(result["id"].(float64))

	// Confirm rule exists.
	listResp, _ := ss.Client().Get(ss.URL + "/orchestration/simplestore/rules")
	var listBody struct{ Count int }
	json.NewDecoder(listResp.Body).Decode(&listBody)
	listResp.Body.Close()
	if listBody.Count != 1 {
		t.Fatalf("expected 1 rule, got %d", listBody.Count)
	}

	// Delete and confirm gone.
	deleteAt(t, ss, "/orchestration/simplestore/rules/"+itoa(id), http.StatusOK)

	results := decodeOrchResponse(t, post(t, ss, "/orchestration/simplestore", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 0 {
		t.Errorf("expected 0 results after rule deletion, got %d", len(results))
	}
}

// ── FlexibleStore Orchestration ───────────────────────────────────────────────

func TestE2EFlexibleStorePriorityOrdering(t *testing.T) {
	fs := startFlexibleStore(t); defer fs.Close()

	// Create rules in reverse priority order.
	for _, rule := range []map[string]any{
		{"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
			"provider": map[string]any{"systemName": "fallback", "address": "10.0.0.2", "port": 9002},
			"serviceUri": "/temperature", "interfaces": []string{"HTTP"}, "priority": 2},
		{"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
			"provider": map[string]any{"systemName": "preferred", "address": "10.0.0.1", "port": 9001},
			"serviceUri": "/temperature", "interfaces": []string{"HTTP"}, "priority": 1},
	} {
		mustPost(t, fs, "/orchestration/flexiblestore/rules", rule, http.StatusCreated)
	}

	results := decodeOrchResponse(t, post(t, fs, "/orchestration/flexiblestore", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0]["provider"].(map[string]any)["systemName"] != "preferred" {
		t.Errorf("expected 'preferred' first (priority 1), got %v", results[0]["provider"])
	}
}

func TestE2EFlexibleStoreMetadataFiltering(t *testing.T) {
	fs := startFlexibleStore(t); defer fs.Close()

	mustPost(t, fs, "/orchestration/flexiblestore/rules", map[string]any{
		"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
		"provider": map[string]any{"systemName": "eu-sensor", "address": "10.0.0.1", "port": 9001},
		"serviceUri": "/temperature", "interfaces": []string{"HTTP"}, "priority": 1,
		"metadataFilter": map[string]string{"region": "eu"},
	}, http.StatusCreated)
	mustPost(t, fs, "/orchestration/flexiblestore/rules", map[string]any{
		"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
		"provider": map[string]any{"systemName": "global-sensor", "address": "10.0.0.2", "port": 9002},
		"serviceUri": "/temperature", "interfaces": []string{"HTTP"}, "priority": 2,
	}, http.StatusCreated)

	// EU request matches both rules (global filter is an empty subset of any metadata).
	euResults := decodeOrchResponse(t, post(t, fs, "/orchestration/flexiblestore", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service", "metadata": map[string]string{"region": "eu"}},
	}))
	if len(euResults) != 2 {
		t.Fatalf("eu request: expected 2 results, got %d", len(euResults))
	}
	if euResults[0]["provider"].(map[string]any)["systemName"] != "eu-sensor" {
		t.Errorf("expected eu-sensor first, got %v", euResults[0]["provider"])
	}

	// No-metadata request matches only the global rule.
	noMetaResults := decodeOrchResponse(t, post(t, fs, "/orchestration/flexiblestore", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(noMetaResults) != 1 {
		t.Fatalf("no-metadata request: expected 1 result, got %d", len(noMetaResults))
	}
	if noMetaResults[0]["provider"].(map[string]any)["systemName"] != "global-sensor" {
		t.Errorf("expected global-sensor, got %v", noMetaResults[0]["provider"])
	}
}

// ── ConsumerAuthorization lifecycle ──────────────────────────────────────────

func TestE2EAuthorizationRuleLifecycle(t *testing.T) {
	ca := startCA(t); defer ca.Close()

	result := mustPost(t, ca, "/authorization/grant", map[string]string{
		"consumerSystemName": "consumer-app",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	}, http.StatusCreated)
	id := int(result["id"].(float64))

	verifyResult := mustPost(t, ca, "/authorization/verify", map[string]string{
		"consumerSystemName": "consumer-app",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	}, http.StatusOK)
	if verifyResult["authorized"] != true {
		t.Error("expected authorized=true after grant")
	}

	deleteAt(t, ca, "/authorization/revoke/"+itoa(id), http.StatusOK)

	verifyAfterRevoke := mustPost(t, ca, "/authorization/verify", map[string]string{
		"consumerSystemName": "consumer-app",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	}, http.StatusOK)
	if verifyAfterRevoke["authorized"] != false {
		t.Error("expected authorized=false after revoke")
	}
}

func TestE2EDuplicateGrantRejected(t *testing.T) {
	ca := startCA(t); defer ca.Close()

	body := map[string]string{
		"consumerSystemName": "consumer-app",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	}
	mustPost(t, ca, "/authorization/grant", body, http.StatusCreated)
	resp := post(t, ca, "/authorization/grant", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 for duplicate grant, got %d", resp.StatusCode)
	}
}

// ── Full multi-system flow ────────────────────────────────────────────────────

// TestE2EFullFlow is the complete Arrowhead workflow:
// provider registers → admin grants auth → consumer orchestrates → provider returned.
func TestE2EFullFlow(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", true, false)
	defer dynOrch.Close()

	registerService(t, sr, "sensor-1", "temperature-service")
	grantRule(t, ca, "consumer-app", "sensor-1", "temperature-service")

	results := decodeOrchResponse(t, post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 result in full flow, got %d", len(results))
	}
	if results[0]["provider"].(map[string]any)["systemName"] != "sensor-1" {
		t.Errorf("unexpected provider: %v", results[0]["provider"])
	}
}

// TestE2EUnregisterClearsOrchestration verifies that after a provider unregisters,
// dynamic orchestration no longer returns it.
func TestE2EUnregisterClearsOrchestration(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", false, false)
	defer dynOrch.Close()

	registerService(t, sr, "sensor-1", "temperature-service")

	before := decodeOrchResponse(t, post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(before) != 1 {
		t.Fatalf("expected 1 before unregister, got %d", len(before))
	}

	deleteJSON(t, sr, "/serviceregistry/unregister",
		`{"serviceDefinition":"temperature-service","providerSystem":{"systemName":"sensor-1","address":"10.0.0.1","port":9000},"version":1}`,
		http.StatusOK)

	after := decodeOrchResponse(t, post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(after) != 0 {
		t.Errorf("expected 0 after unregister, got %d", len(after))
	}
}

// ── Identity check integration ────────────────────────────────────────────────

// TestE2EIdentityCheckBlocksWithoutToken verifies that when ENABLE_IDENTITY_CHECK=true,
// a request without an Authorization header is rejected with 401.
func TestE2EIdentityCheckBlocksWithoutToken(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	auth := startAuth(t); defer auth.Close()
	registerService(t, sr, "sensor-1", "temperature-service")

	dynOrch := startDynOrch(t, sr.URL, "", auth.URL, false, true)
	defer dynOrch.Close()

	resp := post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", resp.StatusCode)
	}
}

// TestE2EIdentityCheckAllowsWithValidToken verifies the full identity-verified flow:
// consumer logs in → receives token → presents token → orchestrator verifies identity
// → uses verified systemName for CA check → authorized provider returned.
func TestE2EIdentityCheckAllowsWithValidToken(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	auth := startAuth(t); defer auth.Close()
	ca := startCA(t); defer ca.Close()

	registerService(t, sr, "sensor-1", "temperature-service")
	grantRule(t, ca, "consumer-app", "sensor-1", "temperature-service")

	// Consumer logs in and gets a token.
	token := login(t, auth, "consumer-app")

	dynOrch := startDynOrch(t, sr.URL, ca.URL, auth.URL, true, true)
	defer dynOrch.Close()

	resp := postWithToken(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}, token)
	results := decodeOrchResponse(t, resp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["provider"].(map[string]any)["systemName"] != "sensor-1" {
		t.Errorf("unexpected provider: %v", results[0]["provider"])
	}
}

// TestE2EIdentityCheckPreventsImpersonation verifies that a consumer cannot
// impersonate another system: the verified token identity is used for CA checks,
// not the self-reported requesterSystem.systemName.
func TestE2EIdentityCheckPreventsImpersonation(t *testing.T) {
	sr := startSR(t); defer sr.Close()
	auth := startAuth(t); defer auth.Close()
	ca := startCA(t); defer ca.Close()

	registerService(t, sr, "sensor-1", "temperature-service")
	// Only "consumer-app" is authorized — not "impersonator".
	grantRule(t, ca, "consumer-app", "sensor-1", "temperature-service")

	// "consumer-app" logs in and gets a token.
	token := login(t, auth, "consumer-app")

	dynOrch := startDynOrch(t, sr.URL, ca.URL, auth.URL, true, true)
	defer dynOrch.Close()

	// Request claims to be "impersonator" but presents consumer-app's token.
	// The orchestrator should use "consumer-app" (from token) for CA check → authorized.
	resp := postWithToken(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "impersonator", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}, token)
	results := decodeOrchResponse(t, resp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (token identity used), got %d", len(results))
	}

	// Conversely: a system with no token at all should be blocked.
	noTokenResp := post(t, dynOrch, "/orchestration/dynamic", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	})
	noTokenResp.Body.Close()
	if noTokenResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", noTokenResp.StatusCode)
	}
}
