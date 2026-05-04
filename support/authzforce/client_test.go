package authzforce

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── BuildPolicy / XACML generation ───────────────────────────────────────────

func TestBuildPolicy_empty(t *testing.T) {
	xml := BuildPolicy("urn:arrowhead:test", "1", nil)
	if !strings.Contains(xml, `PolicySetId="urn:arrowhead:test"`) {
		t.Fatal("missing PolicySetId")
	}
	if !strings.Contains(xml, `Version="1"`) {
		t.Fatal("missing Version")
	}
	if !strings.Contains(xml, "deny-unless-permit") {
		t.Fatal("missing combining algorithm")
	}
}

func TestBuildPolicy_withGrants(t *testing.T) {
	grants := []Grant{
		{Consumer: "consumer-1", Service: "telemetry"},
		{Consumer: "consumer-2", Service: "telemetry"},
	}
	xml := BuildPolicy("urn:arrowhead:test", "2", grants)
	if !strings.Contains(xml, "consumer-1") {
		t.Fatal("grant for consumer-1 missing")
	}
	if !strings.Contains(xml, "consumer-2") {
		t.Fatal("grant for consumer-2 missing")
	}
	if !strings.Contains(xml, "telemetry") {
		t.Fatal("service missing from policy")
	}
}

// ── parseDecision ─────────────────────────────────────────────────────────────

func TestParseDecision_permit(t *testing.T) {
	body := []byte(`<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Permit</Decision>` +
		`<Status><StatusCode Value="urn:oasis:names:tc:xacml:1.0:status:ok"/></Status>` +
		`</Result></Response>`)
	d, err := parseDecision(body)
	if err != nil {
		t.Fatalf("parseDecision: %v", err)
	}
	if d != "Permit" {
		t.Fatalf("expected Permit, got %q", d)
	}
}

func TestParseDecision_deny(t *testing.T) {
	body := []byte(`<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Deny</Decision></Result></Response>`)
	d, err := parseDecision(body)
	if err != nil {
		t.Fatalf("parseDecision: %v", err)
	}
	if d != "Deny" {
		t.Fatalf("expected Deny, got %q", d)
	}
}

// ── Client.Decide (mock HTTP server) ─────────────────────────────────────────

func permitResponse() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Permit</Decision></Result></Response>`
}

func denyResponse() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Deny</Decision></Result></Response>`
}

func TestDecide_permit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(permitResponse()))
	}))
	defer srv.Close()

	c := New(srv.URL)
	ok, err := c.Decide("test-domain", "consumer-1", "telemetry", "subscribe")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !ok {
		t.Fatal("expected Permit")
	}
}

func TestDecide_deny(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(denyResponse()))
	}))
	defer srv.Close()

	c := New(srv.URL)
	ok, err := c.Decide("test-domain", "consumer-X", "telemetry", "subscribe")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if ok {
		t.Fatal("expected Deny")
	}
}

func TestDecide_pdpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Decide("test-domain", "consumer-1", "telemetry", "subscribe")
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

// ── buildXACMLRequest ─────────────────────────────────────────────────────────

func TestBuildXACMLRequest_containsAllFields(t *testing.T) {
	req := buildXACMLRequest("consumer-1", "telemetry", "subscribe")
	for _, want := range []string{"consumer-1", "telemetry", "subscribe", "Request"} {
		if !strings.Contains(req, want) {
			t.Fatalf("buildXACMLRequest: missing %q", want)
		}
	}
}
