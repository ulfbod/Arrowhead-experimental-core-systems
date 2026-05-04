// authzforce-server — lightweight AuthzForce-compatible XACML PDP/PAP server.
//
// Implements the subset of the AuthzForce CE REST API used by the arrowhead/authzforce
// Go client:
//
//   POST /authzforce-ce/domains                    — create domain
//   GET  /authzforce-ce/domains?externalId=X       — find domain by external ID
//   PUT  /authzforce-ce/domains/{id}/pap/policies  — upload XACML PolicySet (PAP)
//   PUT  /authzforce-ce/domains/{id}/pap/pdp.properties — set root policy ref (no-op)
//   POST /authzforce-ce/domains/{id}/pdp           — evaluate XACML Request (PDP)
//   GET  /health                                   — liveness probe
//
// The PDP evaluates requests using grants parsed from the PolicySet.  Grants are
// encoded in PolicyId attributes as "urn:arrowhead:grant:{consumer}:{service}".
// A request is Permitted iff the (subject, resource) pair matches a stored grant.
//
// Environment variables:
//
//	PORT   HTTP port (default: 8080)
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── Domain store ───────────────────────────────────────────────────────────────

type domain struct {
	id         string
	externalID string
	grants     map[[2]string]bool // {consumer, service} → true
}

var (
	mu      sync.RWMutex
	domains = map[string]*domain{}  // id → domain
	byExt   = map[string]*domain{}  // externalID → domain
)

func newUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff)
}

// ── Policy parsing ─────────────────────────────────────────────────────────────

// grantRe matches PolicyId="urn:arrowhead:grant:{consumer}:{service}" in a PolicySet.
// We generate this format in arrowhead/authzforce.BuildPolicy.
var grantRe = regexp.MustCompile(`PolicyId="urn:arrowhead:grant:([^:]+):([^"]+)"`)

// parseGrants extracts (consumer, service) pairs from our PolicySet XML format.
func parseGrants(policyXML string) map[[2]string]bool {
	grants := map[[2]string]bool{}
	for _, m := range grantRe.FindAllStringSubmatch(policyXML, -1) {
		grants[[2]string{m[1], m[2]}] = true
	}
	return grants
}

// ── XACML Request parsing ──────────────────────────────────────────────────────

// attrValueRe extracts AttributeValue text content.
// The request format is produced by arrowhead/authzforce.buildXACMLRequest:
// values appear in order: subject, resource, action.
var attrValueRe = regexp.MustCompile(`<AttributeValue[^>]*>([^<]+)</AttributeValue>`)

func parseXACMLRequest(body string) (subject, resource string) {
	matches := attrValueRe.FindAllStringSubmatch(body, -1)
	if len(matches) >= 1 {
		subject = matches[0][1]
	}
	if len(matches) >= 2 {
		resource = matches[1][1]
	}
	return
}

// ── XML responses ──────────────────────────────────────────────────────────────

func linkResponse(domainID string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><link xmlns="http://www.w3.org/2005/Atom" href="/authzforce-ce/domains/%s"/>`, domainID)
}

func resourcesResponse(domainID string) string {
	if domainID == "" {
		return `<?xml version="1.0" encoding="UTF-8"?><resources xmlns="http://authzforce.github.io/rest-api-model/xmlns/authz/5"/>`
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><resources xmlns="http://authzforce.github.io/rest-api-model/xmlns/authz/5"><link xmlns="http://www.w3.org/2005/Atom" href="/authzforce-ce/domains/%s"/></resources>`, domainID)
}

func xacmlResponse(decision string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"><Result><Decision>%s</Decision></Result></Response>`, decision)
}

func xmlReply(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/xml;charset=UTF-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// ── Request parsing helpers ───────────────────────────────────────────────────

func readBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

// parseExternalID extracts <externalId>...</externalId> from domain properties XML.
var extIDRe = regexp.MustCompile(`<externalId>([^<]+)</externalId>`)

func parseExternalID(body string) string {
	m := extIDRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ── HTTP handlers ──────────────────────────────────────────────────────────────

func handleDomains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		extID := r.URL.Query().Get("externalId")
		mu.RLock()
		d := byExt[extID]
		mu.RUnlock()
		id := ""
		if d != nil {
			id = d.id
		}
		xmlReply(w, http.StatusOK, resourcesResponse(id))

	case http.MethodPost:
		body := readBody(r)
		extID := parseExternalID(body)

		mu.Lock()
		// Idempotent: return existing domain if same externalID.
		if d, ok := byExt[extID]; ok {
			mu.Unlock()
			xmlReply(w, http.StatusCreated, linkResponse(d.id))
			return
		}
		d := &domain{id: newUUID(), externalID: extID, grants: map[[2]string]bool{}}
		domains[d.id] = d
		byExt[extID] = d
		mu.Unlock()
		log.Printf("[authzforce] created domain id=%s externalId=%s", d.id, extID)
		xmlReply(w, http.StatusCreated, linkResponse(d.id))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleDomainSub(w http.ResponseWriter, r *http.Request) {
	// Path: /authzforce-ce/domains/{id}/...
	rest := strings.TrimPrefix(r.URL.Path, "/authzforce-ce/domains/")
	parts := strings.SplitN(rest, "/", -1)
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "missing domain id", http.StatusBadRequest)
		return
	}
	domainID := parts[0]

	mu.RLock()
	d, ok := domains[domainID]
	mu.RUnlock()
	if !ok {
		http.Error(w, "domain not found", http.StatusNotFound)
		return
	}

	// /domains/{id}/pap/policies
	if len(parts) >= 3 && parts[1] == "pap" && parts[2] == "policies" {
		if r.Method != http.MethodPut {
			http.Error(w, "PUT required", http.StatusMethodNotAllowed)
			return
		}
		body := readBody(r)
		grants := parseGrants(body)
		mu.Lock()
		d.grants = grants
		mu.Unlock()
		log.Printf("[authzforce] domain=%s policy updated: %d grants", domainID, len(grants))
		xmlReply(w, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><link xmlns="http://www.w3.org/2005/Atom" href="/authzforce-ce/domains/`+domainID+`/pap/policies"/>`)
		return
	}

	// /domains/{id}/pap/pdp.properties
	if len(parts) >= 3 && parts[1] == "pap" && parts[2] == "pdp.properties" {
		if r.Method != http.MethodPut {
			http.Error(w, "PUT required", http.StatusMethodNotAllowed)
			return
		}
		// No-op: we always use whatever policy was last uploaded.
		xmlReply(w, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><pdpProperties xmlns="http://authzforce.github.io/rest-api-model/xmlns/authz/5"/>`)
		return
	}

	// /domains/{id}/pdp
	if len(parts) >= 2 && parts[1] == "pdp" {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		body := readBody(r)
		subject, resource := parseXACMLRequest(body)

		mu.RLock()
		permitted := d.grants[[2]string{subject, resource}]
		mu.RUnlock()

		decision := "Deny"
		if permitted {
			decision = "Permit"
		}
		log.Printf("[authzforce] domain=%s pdp subject=%q resource=%q → %s", domainID, subject, resource, decision)
		xmlReply(w, http.StatusOK, xacmlResponse(decision))
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

// ── Main ───────────────────────────────────────────────────────────────────────

func main() {
	port := envOr("PORT", "8080")

	mux := http.NewServeMux()

	// Exact match for the domains collection endpoint.
	mux.HandleFunc("/authzforce-ce/domains", handleDomains)

	// Prefix match for per-domain sub-resources.
	mux.HandleFunc("/authzforce-ce/domains/", handleDomainSub)

	// Health probes — both root and the /authzforce-ce/ prefix.
	// The nginx proxy rewrites /api/authzforce/health → /authzforce-ce/health,
	// so both paths must return 200 or the dashboard shows AuthzForce as "down".
	healthHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/authzforce-ce/health", healthHandler)

	log.Printf("[authzforce-server] XACML PDP/PAP listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

