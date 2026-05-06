package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// resetState clears global domain store between tests.
func resetState() {
	mu.Lock()
	defer mu.Unlock()
	domains = map[string]*domain{}
	byExt = map[string]*domain{}
}

// ── parseGrants ───────────────────────────────────────────────────────────────

func TestParseGrants_empty(t *testing.T) {
	got := parseGrants("<PolicySet/>")
	if len(got) != 0 {
		t.Errorf("expected 0 grants, got %d", len(got))
	}
}

func TestParseGrants_single(t *testing.T) {
	xml := `<Policy PolicyId="urn:arrowhead:grant:consumer-1:telemetry"/>`
	got := parseGrants(xml)
	if !got[[2]string{"consumer-1", "telemetry"}] {
		t.Errorf("expected grant (consumer-1, telemetry), not found in %v", got)
	}
}

func TestParseGrants_multiple(t *testing.T) {
	xml := `<Policy PolicyId="urn:arrowhead:grant:consumer-1:telemetry"/>` +
		`<Policy PolicyId="urn:arrowhead:grant:consumer-2:events"/>` +
		`<Policy PolicyId="urn:arrowhead:grant:consumer-3:telemetry"/>`
	got := parseGrants(xml)
	if len(got) != 3 {
		t.Fatalf("expected 3 grants, got %d: %v", len(got), got)
	}
	if !got[[2]string{"consumer-2", "events"}] {
		t.Error("missing (consumer-2, events)")
	}
}

// ── parseXACMLRequest ─────────────────────────────────────────────────────────

func TestParseXACMLRequest_both(t *testing.T) {
	body := `<AttributeValue DataType="xs:string">my-consumer</AttributeValue>` +
		`<AttributeValue DataType="xs:string">my-service</AttributeValue>` +
		`<AttributeValue DataType="xs:string">invoke</AttributeValue>`
	subj, res := parseXACMLRequest(body)
	if subj != "my-consumer" {
		t.Errorf("subject: got %q, want %q", subj, "my-consumer")
	}
	if res != "my-service" {
		t.Errorf("resource: got %q, want %q", res, "my-service")
	}
}

func TestParseXACMLRequest_empty(t *testing.T) {
	subj, res := parseXACMLRequest("<Request/>")
	if subj != "" || res != "" {
		t.Errorf("expected empty strings, got %q %q", subj, res)
	}
}

// ── parseExternalID ───────────────────────────────────────────────────────────

func TestParseExternalID_present(t *testing.T) {
	body := `<ns2:domainProperties><externalId>arrowhead-exp5</externalId></ns2:domainProperties>`
	got := parseExternalID(body)
	if got != "arrowhead-exp5" {
		t.Errorf("got %q, want arrowhead-exp5", got)
	}
}

func TestParseExternalID_absent(t *testing.T) {
	got := parseExternalID("<empty/>")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	for _, path := range []string{"/health", "/authzforce-ce/health"} {
		mux := http.NewServeMux()
		healthHandler := func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		}
		mux.HandleFunc("/health", healthHandler)
		mux.HandleFunc("/authzforce-ce/health", healthHandler)

		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "ok") {
			t.Errorf("%s: body missing 'ok': %s", path, rec.Body.String())
		}
	}
}

// ── GET /authzforce-ce/domains ────────────────────────────────────────────────

func TestGetDomains_notFound(t *testing.T) {
	resetState()

	req := httptest.NewRequest(http.MethodGet, "/authzforce-ce/domains?externalId=unknown", nil)
	rec := httptest.NewRecorder()
	handleDomains(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	// Empty resources response — no href link.
	body := rec.Body.String()
	if strings.Contains(body, "href") {
		t.Errorf("unexpected href in response: %s", body)
	}
}

func TestGetDomains_found(t *testing.T) {
	resetState()

	// Create a domain first.
	createReq := httptest.NewRequest(http.MethodPost, "/authzforce-ce/domains",
		strings.NewReader(`<ns2:domainProperties><externalId>test-ext</externalId></ns2:domainProperties>`))
	createRec := httptest.NewRecorder()
	handleDomains(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create domain: got %d, want 201", createRec.Code)
	}

	// Now look it up.
	req := httptest.NewRequest(http.MethodGet, "/authzforce-ce/domains?externalId=test-ext", nil)
	rec := httptest.NewRecorder()
	handleDomains(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "href") {
		t.Errorf("expected href in response: %s", rec.Body.String())
	}
}

// ── POST /authzforce-ce/domains ───────────────────────────────────────────────

func TestCreateDomain(t *testing.T) {
	resetState()

	body := `<ns2:domainProperties><externalId>arrowhead-test</externalId></ns2:domainProperties>`
	req := httptest.NewRequest(http.MethodPost, "/authzforce-ce/domains", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleDomains(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("got %d, want 201", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "href") {
		t.Errorf("response missing href: %s", rec.Body.String())
	}
}

func TestCreateDomain_idempotent(t *testing.T) {
	resetState()

	body := `<ns2:domainProperties><externalId>arrowhead-idempotent</externalId></ns2:domainProperties>`

	req1 := httptest.NewRequest(http.MethodPost, "/authzforce-ce/domains", strings.NewReader(body))
	rec1 := httptest.NewRecorder()
	handleDomains(rec1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/authzforce-ce/domains", strings.NewReader(body))
	rec2 := httptest.NewRecorder()
	handleDomains(rec2, req2)

	if rec1.Code != http.StatusCreated || rec2.Code != http.StatusCreated {
		t.Errorf("both calls should return 201; got %d and %d", rec1.Code, rec2.Code)
	}
	// Both responses should contain the same domain ID href.
	href1 := extractHrefValue(rec1.Body.String())
	href2 := extractHrefValue(rec2.Body.String())
	if href1 == "" || href1 != href2 {
		t.Errorf("idempotent create: got different hrefs %q vs %q", href1, href2)
	}
}

// extractHrefValue pulls the href="..." value from an XML body.
func extractHrefValue(body string) string {
	const prefix = `href="`
	i := strings.Index(body, prefix)
	if i < 0 {
		return ""
	}
	rest := body[i+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// ── PUT /authzforce-ce/domains/{id}/pap/policies ──────────────────────────────

func TestPutPolicy_updatesGrants(t *testing.T) {
	resetState()

	// Create domain.
	createBody := `<ns2:domainProperties><externalId>exp-put</externalId></ns2:domainProperties>`
	createReq := httptest.NewRequest(http.MethodPost, "/authzforce-ce/domains", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	handleDomains(createRec, createReq)
	domainID := lastPathSegment(extractHrefValue(createRec.Body.String()))

	// Upload a policy with two grants.
	policy := `<PolicySet>` +
		`<Policy PolicyId="urn:arrowhead:grant:consumer-1:telemetry"/>` +
		`<Policy PolicyId="urn:arrowhead:grant:consumer-2:telemetry"/>` +
		`</PolicySet>`
	putReq := httptest.NewRequest(http.MethodPut,
		"/authzforce-ce/domains/"+domainID+"/pap/policies",
		strings.NewReader(policy))
	putRec := httptest.NewRecorder()
	handleDomainSub(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT policy: got %d, want 200; body: %s", putRec.Code, putRec.Body.String())
	}

	// Verify grants are stored.
	mu.RLock()
	d := domains[domainID]
	mu.RUnlock()
	if d == nil {
		t.Fatal("domain not found after PUT")
	}
	if !d.grants[[2]string{"consumer-1", "telemetry"}] {
		t.Error("consumer-1 grant not stored")
	}
	if !d.grants[[2]string{"consumer-2", "telemetry"}] {
		t.Error("consumer-2 grant not stored")
	}
}

// ── POST /authzforce-ce/domains/{id}/pdp ──────────────────────────────────────

func xacmlDecideRequest(consumer, service string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Request xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Attributes Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject">` +
		`<Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:subject:subject-id">` +
		`<AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">` + consumer + `</AttributeValue>` +
		`</Attribute></Attributes>` +
		`<Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">` +
		`<Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:resource:resource-id">` +
		`<AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">` + service + `</AttributeValue>` +
		`</Attribute></Attributes>` +
		`</Request>`
}

func setupDomainWithGrants(t *testing.T) string {
	t.Helper()
	resetState()

	createBody := `<ns2:domainProperties><externalId>exp-pdp</externalId></ns2:domainProperties>`
	createReq := httptest.NewRequest(http.MethodPost, "/authzforce-ce/domains", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	handleDomains(createRec, createReq)
	domainID := lastPathSegment(extractHrefValue(createRec.Body.String()))

	policy := `<PolicySet>` +
		`<Policy PolicyId="urn:arrowhead:grant:consumer-allowed:telemetry"/>` +
		`</PolicySet>`
	putReq := httptest.NewRequest(http.MethodPut,
		"/authzforce-ce/domains/"+domainID+"/pap/policies",
		strings.NewReader(policy))
	putRec := httptest.NewRecorder()
	handleDomainSub(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT policy: %d", putRec.Code)
	}
	return domainID
}

func TestPDP_permit(t *testing.T) {
	domainID := setupDomainWithGrants(t)

	req := httptest.NewRequest(http.MethodPost,
		"/authzforce-ce/domains/"+domainID+"/pdp",
		strings.NewReader(xacmlDecideRequest("consumer-allowed", "telemetry")))
	rec := httptest.NewRecorder()
	handleDomainSub(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Permit") {
		t.Errorf("expected Permit in response: %s", rec.Body.String())
	}
}

func TestPDP_deny(t *testing.T) {
	domainID := setupDomainWithGrants(t)

	req := httptest.NewRequest(http.MethodPost,
		"/authzforce-ce/domains/"+domainID+"/pdp",
		strings.NewReader(xacmlDecideRequest("consumer-denied", "telemetry")))
	rec := httptest.NewRecorder()
	handleDomainSub(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Deny") {
		t.Errorf("expected Deny in response: %s", rec.Body.String())
	}
}

func TestPDP_unknownDomain(t *testing.T) {
	resetState()

	req := httptest.NewRequest(http.MethodPost,
		"/authzforce-ce/domains/nonexistent-id/pdp",
		strings.NewReader(xacmlDecideRequest("c", "s")))
	rec := httptest.NewRecorder()
	handleDomainSub(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// lastPathSegment returns the last "/" segment of a path string.
func lastPathSegment(s string) string {
	s = strings.TrimRight(s, "/")
	i := strings.LastIndex(s, "/")
	if i < 0 {
		return s
	}
	return s[i+1:]
}
