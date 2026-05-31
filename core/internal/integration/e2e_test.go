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
	"strings"
	"testing"
	"time"

	srapi "arrowhead/core/internal/api"
	blclient "arrowhead/core/internal/blacklist/client"
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
		Results []map[string]any `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Results
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// ── System factories ──────────────────────────────────────────────────────────

func startSR(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(srapi.NewHandler(srsvc.NewRegistryService(srrepo.NewMemoryRepository()), blclient.NopClient{}))
}

// startAH5SR starts a ServiceRegistry that serves the AH5 API endpoints.
// DynamicOrchestrator calls POST /serviceregistry/service-discovery/lookup,
// which is only available on the AH5 handler.
func startAH5SR(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(srapi.NewAH5Handler(srsvc.NewAH5RegistryService(srrepo.NewAH5Store()), "", "", ""))
}

func startAuth(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(authapi.NewHandler(authsvc.NewAuthService(authrepo.NewMemoryRepository(), time.Hour), ""))
}

func startCA(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(caapi.NewHandler(casvc.NewAuthService(carepo.NewMemoryRepository()), "", blclient.NopClient{}))
}

func startDynOrch(t *testing.T, srURL, caURL, authSysURL string, checkAuth, checkIdentity bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(dynapi.NewHandler(dynsvc.NewDynamicOrchestrator(srURL, caURL, authSysURL, checkAuth, checkIdentity), ""))
}

func startSimpleStore(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(ssapi.NewHandler(sssvc.NewSimpleStoreOrchestrator(ssrepo.NewMemoryRepository()), ""))
}

func startFlexibleStore(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(fsapi.NewHandler(fssvc.NewFlexibleStoreOrchestrator(fsrepo.NewMemoryRepository()), ""))
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

// registerServiceAH5 registers a service via the AH5 endpoint and returns its instanceId.
// systemName must be PascalCase (e.g. "Sensor1"), serviceDef must be camelCase (e.g. "temperatureService").
func registerServiceAH5(t *testing.T, sr *httptest.Server, systemName, serviceDef string) string {
	t.Helper()
	result := mustPost(t, sr, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName":            systemName,
		"serviceDefinitionName": serviceDef,
		"version":               "1.0.0",
	}, http.StatusCreated)
	id, _ := result["instanceId"].(string)
	return id
}

func grantRule(t *testing.T, ca *httptest.Server, consumer, provider, target string) {
	t.Helper()
	mustPost(t, ca, "/consumerauthorization/authorization/grant", map[string]any{
		"provider":   provider,
		"targetType": "SERVICE_DEF",
		"target":     target,
		"defaultPolicy": map[string]any{
			"policyType": "WHITELIST",
			"policyList": []string{consumer},
		},
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
	sr := startAH5SR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	registerServiceAH5(t, sr, "Sensor1", "temperatureService")

	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", false, false)
	defer dynOrch.Close()

	results := decodeOrchResponse(t, post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["providerName"].(string) != "Sensor1" {
		t.Errorf("unexpected provider: %v", results[0]["providerName"])
	}
}

func TestE2EDynamicOrchestrationWithAuth(t *testing.T) {
	sr := startAH5SR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()

	registerServiceAH5(t, sr, "Sensor1", "temperatureService")
	registerServiceAH5(t, sr, "Sensor2", "temperatureService")
	grantRule(t, ca, "ConsumerApp", "Sensor1", "temperatureService")
	// Sensor2 has no grant.

	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", true, false)
	defer dynOrch.Close()

	results := decodeOrchResponse(t, post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 authorized result, got %d", len(results))
	}
	if results[0]["providerName"].(string) != "Sensor1" {
		t.Errorf("expected Sensor1 (authorized), got %v", results[0]["providerName"])
	}
}

func TestE2EDynamicOrchestrationNoGrantNoResults(t *testing.T) {
	sr := startAH5SR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	registerServiceAH5(t, sr, "Sensor1", "temperatureService")
	// No auth rules granted.

	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", true, false)
	defer dynOrch.Close()

	results := decodeOrchResponse(t, post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}))
	if len(results) != 0 {
		t.Errorf("expected 0 results (no auth rules), got %d", len(results))
	}
}

// ── SimpleStore Orchestration ─────────────────────────────────────────────────

func TestE2ESimpleStoreOrchestration(t *testing.T) {
	ss := startSimpleStore(t); defer ss.Close()

	mustPost(t, ss, "/serviceorchestration/orchestration/simplestore/rules", map[string]any{
		"consumerSystemName": "consumer-app",
		"serviceDefinition":  "temperature-service",
		"provider":           map[string]any{"systemName": "sensor-1", "address": "10.0.0.1", "port": 9000},
		"serviceUri":         "/temperature",
		"interfaces":         []string{"HTTP-INSECURE-JSON"},
	}, http.StatusCreated)

	results := decodeOrchResponse(t, post(t, ss, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["providerName"].(string) != "sensor-1" {
		t.Errorf("unexpected provider: %v", results[0]["providerName"])
	}
}

func TestE2ESimpleStoreRuleLifecycle(t *testing.T) {
	ss := startSimpleStore(t); defer ss.Close()

	result := mustPost(t, ss, "/serviceorchestration/orchestration/simplestore/rules", map[string]any{
		"consumerSystemName": "consumer-app",
		"serviceDefinition":  "temperature-service",
		"provider":           map[string]any{"systemName": "sensor-1", "address": "10.0.0.1", "port": 9000},
		"serviceUri":         "/temperature",
		"interfaces":         []string{"HTTP"},
	}, http.StatusCreated)
	id := result["id"].(string)

	// Confirm rule exists.
	listResp, _ := ss.Client().Get(ss.URL + "/serviceorchestration/orchestration/simplestore/rules")
	var listBody struct{ Count int }
	json.NewDecoder(listResp.Body).Decode(&listBody)
	listResp.Body.Close()
	if listBody.Count != 1 {
		t.Fatalf("expected 1 rule, got %d", listBody.Count)
	}

	// Delete and confirm gone.
	deleteAt(t, ss, "/serviceorchestration/orchestration/simplestore/rules/"+id, http.StatusNoContent)

	results := decodeOrchResponse(t, post(t, ss, "/serviceorchestration/orchestration/pull", map[string]any{
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
		mustPost(t, fs, "/serviceorchestration/orchestration/flexiblestore/rules", rule, http.StatusCreated)
	}

	results := decodeOrchResponse(t, post(t, fs, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0]["providerName"].(string) != "preferred" {
		t.Errorf("expected 'preferred' first (priority 1), got %v", results[0]["providerName"])
	}
}

func TestE2EFlexibleStoreMetadataFiltering(t *testing.T) {
	fs := startFlexibleStore(t); defer fs.Close()

	mustPost(t, fs, "/serviceorchestration/orchestration/flexiblestore/rules", map[string]any{
		"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
		"provider": map[string]any{"systemName": "eu-sensor", "address": "10.0.0.1", "port": 9001},
		"serviceUri": "/temperature", "interfaces": []string{"HTTP"}, "priority": 1,
		"metadataFilter": map[string]string{"region": "eu"},
	}, http.StatusCreated)
	mustPost(t, fs, "/serviceorchestration/orchestration/flexiblestore/rules", map[string]any{
		"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
		"provider": map[string]any{"systemName": "global-sensor", "address": "10.0.0.2", "port": 9002},
		"serviceUri": "/temperature", "interfaces": []string{"HTTP"}, "priority": 2,
	}, http.StatusCreated)

	// EU request matches both rules (global filter is an empty subset of any metadata).
	euResults := decodeOrchResponse(t, post(t, fs, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service", "metadata": map[string]string{"region": "eu"}},
	}))
	if len(euResults) != 2 {
		t.Fatalf("eu request: expected 2 results, got %d", len(euResults))
	}
	if euResults[0]["providerName"].(string) != "eu-sensor" {
		t.Errorf("expected eu-sensor first, got %v", euResults[0]["providerName"])
	}

	// No-metadata request matches only the global rule.
	noMetaResults := decodeOrchResponse(t, post(t, fs, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}))
	if len(noMetaResults) != 1 {
		t.Fatalf("no-metadata request: expected 1 result, got %d", len(noMetaResults))
	}
	if noMetaResults[0]["providerName"].(string) != "global-sensor" {
		t.Errorf("expected global-sensor, got %v", noMetaResults[0]["providerName"])
	}
}

// ── ConsumerAuthorization lifecycle ──────────────────────────────────────────

func TestE2EAuthorizationRuleLifecycle(t *testing.T) {
	ca := startCA(t)
	defer ca.Close()

	// Grant returns instanceId (not a numeric id).
	result := mustPost(t, ca, "/consumerauthorization/authorization/grant", map[string]any{
		"provider":      "sensor-1",
		"targetType":    "SERVICE_DEF",
		"target":        "temperature-service",
		"defaultPolicy": map[string]any{"policyType": "WHITELIST", "policyList": []string{"consumer-app"}},
	}, http.StatusCreated)
	instanceID, ok := result["instanceId"].(string)
	if !ok || instanceID == "" {
		t.Fatalf("grant response missing instanceId: %v", result)
	}

	// Verify returns plain JSON boolean.
	verifyResp := post(t, ca, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer":   "consumer-app",
		"provider":   "sensor-1",
		"target":     "temperature-service",
		"targetType": "SERVICE_DEF",
	})
	defer verifyResp.Body.Close()
	var authorized bool
	json.NewDecoder(verifyResp.Body).Decode(&authorized)
	if !authorized {
		t.Error("expected authorized=true after grant")
	}

	// Revoke by instanceId (URL-encode the pipes).
	encoded := strings.ReplaceAll(instanceID, "|", "%7C")
	deleteAt(t, ca, "/consumerauthorization/authorization/revoke/"+encoded, http.StatusOK)

	// Verify after revoke.
	verifyResp2 := post(t, ca, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer":   "consumer-app",
		"provider":   "sensor-1",
		"target":     "temperature-service",
		"targetType": "SERVICE_DEF",
	})
	defer verifyResp2.Body.Close()
	var authorized2 bool
	json.NewDecoder(verifyResp2.Body).Decode(&authorized2)
	if authorized2 {
		t.Error("expected authorized=false after revoke")
	}
}

func TestE2EDuplicateGrantRejected(t *testing.T) {
	ca := startCA(t)
	defer ca.Close()

	body := map[string]any{
		"provider":      "sensor-1",
		"targetType":    "SERVICE_DEF",
		"target":        "temperature-service",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	}
	mustPost(t, ca, "/consumerauthorization/authorization/grant", body, http.StatusCreated)
	resp := post(t, ca, "/consumerauthorization/authorization/grant", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 for duplicate grant, got %d", resp.StatusCode)
	}
}

// ── Full multi-system flow ────────────────────────────────────────────────────

// TestE2EFullFlow is the complete Arrowhead workflow:
// provider registers → admin grants auth → consumer orchestrates → provider returned.
func TestE2EFullFlow(t *testing.T) {
	sr := startAH5SR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", true, false)
	defer dynOrch.Close()

	registerServiceAH5(t, sr, "Sensor1", "temperatureService")
	grantRule(t, ca, "ConsumerApp", "Sensor1", "temperatureService")

	results := decodeOrchResponse(t, post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}))
	if len(results) != 1 {
		t.Fatalf("expected 1 result in full flow, got %d", len(results))
	}
	if results[0]["providerName"].(string) != "Sensor1" {
		t.Errorf("unexpected provider: %v", results[0]["providerName"])
	}
}

// TestE2EUnregisterClearsOrchestration verifies that after a provider unregisters,
// dynamic orchestration no longer returns it.
func TestE2EUnregisterClearsOrchestration(t *testing.T) {
	sr := startAH5SR(t); defer sr.Close()
	ca := startCA(t); defer ca.Close()
	dynOrch := startDynOrch(t, sr.URL, ca.URL, "", false, false)
	defer dynOrch.Close()

	instanceID := registerServiceAH5(t, sr, "Sensor1", "temperatureService")

	before := decodeOrchResponse(t, post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}))
	if len(before) != 1 {
		t.Fatalf("expected 1 before unregister, got %d", len(before))
	}

	// AH5 revoke by instanceId.
	encoded := strings.ReplaceAll(instanceID, "|", "%7C")
	deleteAt(t, sr, "/serviceregistry/service-discovery/revoke/"+encoded, http.StatusOK)

	after := decodeOrchResponse(t, post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}))
	if len(after) != 0 {
		t.Errorf("expected 0 after unregister, got %d", len(after))
	}
}

// ── Identity check integration ────────────────────────────────────────────────

// TestE2EIdentityCheckBlocksWithoutToken verifies that when ENABLE_IDENTITY_CHECK=true,
// a request without an Authorization header is rejected with 401.
func TestE2EIdentityCheckBlocksWithoutToken(t *testing.T) {
	sr := startAH5SR(t); defer sr.Close()
	auth := startAuth(t); defer auth.Close()
	registerServiceAH5(t, sr, "Sensor1", "temperatureService")

	dynOrch := startDynOrch(t, sr.URL, "", auth.URL, false, true)
	defer dynOrch.Close()

	resp := post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
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
	sr := startAH5SR(t); defer sr.Close()
	auth := startAuth(t); defer auth.Close()
	ca := startCA(t); defer ca.Close()

	registerServiceAH5(t, sr, "Sensor1", "temperatureService")
	grantRule(t, ca, "ConsumerApp", "Sensor1", "temperatureService")

	// Consumer logs in and gets a token.
	token := login(t, auth, "ConsumerApp")

	dynOrch := startDynOrch(t, sr.URL, ca.URL, auth.URL, true, true)
	defer dynOrch.Close()

	resp := postWithToken(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}, token)
	results := decodeOrchResponse(t, resp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["providerName"].(string) != "Sensor1" {
		t.Errorf("unexpected provider: %v", results[0]["providerName"])
	}
}

// TestE2EIdentityCheckPreventsImpersonation verifies that a consumer cannot
// impersonate another system: the verified token identity is used for CA checks,
// not the self-reported requesterSystem.systemName.
func TestE2EIdentityCheckPreventsImpersonation(t *testing.T) {
	sr := startAH5SR(t); defer sr.Close()
	auth := startAuth(t); defer auth.Close()
	ca := startCA(t); defer ca.Close()

	registerServiceAH5(t, sr, "Sensor1", "temperatureService")
	// Only "ConsumerApp" is authorized — not "Impersonator".
	grantRule(t, ca, "ConsumerApp", "Sensor1", "temperatureService")

	// "ConsumerApp" logs in and gets a token.
	token := login(t, auth, "ConsumerApp")

	dynOrch := startDynOrch(t, sr.URL, ca.URL, auth.URL, true, true)
	defer dynOrch.Close()

	// Request claims to be "Impersonator" but presents ConsumerApp's token.
	// The orchestrator should use "ConsumerApp" (from token) for CA check → authorized.
	resp := postWithToken(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "Impersonator", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}, token)
	results := decodeOrchResponse(t, resp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (token identity used), got %d", len(results))
	}

	// Conversely: a system with no token at all should be blocked.
	noTokenResp := post(t, dynOrch, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	})
	noTokenResp.Body.Close()
	if noTokenResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", noTokenResp.StatusCode)
	}
}

// ── AH5 ServiceRegistry composite instanceId (Step 2 / G13) ──────────────────

// ── AH5 423 Locked on device delete with dependent system (Step 3 / G18) ──────

func TestAH5DeviceRevoke423WithDependentSystem(t *testing.T) {
	ah5SR := httptest.NewServer(srapi.NewAH5Handler(srsvc.NewAH5RegistryService(srrepo.NewAH5Store()), "", "", ""))
	defer ah5SR.Close()

	// Register device
	mustPost(t, ah5SR, "/serviceregistry/device-discovery/register",
		map[string]any{"name": "GW01"}, http.StatusCreated)

	// Register system referencing device
	mustPost(t, ah5SR, "/serviceregistry/system-discovery/register",
		map[string]any{
			"name":       "Sensor1",
			"deviceName": "GW01",
			"addresses":  []map[string]any{{"type": "IP", "address": "192.0.2.1"}},
		}, http.StatusCreated)

	// Attempt to revoke device — expect 423 Locked
	req, _ := http.NewRequest(http.MethodDelete, ah5SR.URL+"/serviceregistry/device-discovery/revoke/GW01", nil)
	resp, err := ah5SR.Client().Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusLocked {
		t.Errorf("expected 423 Locked, got %d", resp.StatusCode)
	}
}

func TestAH5ServiceRegistrationCompositeInstanceID(t *testing.T) {
	ah5SR := httptest.NewServer(srapi.NewAH5Handler(srsvc.NewAH5RegistryService(srrepo.NewAH5Store()), "", "", ""))
	defer ah5SR.Close()

	body, _ := json.Marshal(map[string]any{
		"systemName":            "Provider1",
		"serviceDefinitionName": "temperature",
		"version":               "1.0.0",
	})
	resp, err := ah5SR.Client().Post(
		ah5SR.URL+"/serviceregistry/service-discovery/register",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 201/200, got %d", resp.StatusCode)
	}
	var inst map[string]any
	json.NewDecoder(resp.Body).Decode(&inst)
	id, _ := inst["instanceId"].(string)
	if !strings.Contains(id, "|") {
		t.Errorf("AH5 instanceId should be composite string, got %q", id)
	}
}
