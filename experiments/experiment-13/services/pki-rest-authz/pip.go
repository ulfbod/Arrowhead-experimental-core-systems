// pip.go — PIP (Policy Information Point) HTTP client for pki-rest-authz.
//
// Before calling AuthzForce, the PEP queries PIP to fetch cert-level attributes
// for the subject. If PIP returns 404 or is unreachable, the PEP fails closed.
//
// Decision D1: no PEP-side caching of PIP responses.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// subjectAttrs holds the cert-level attributes returned by PIP.
type subjectAttrs struct {
	CertLevel string
	CertValid bool
}

// pipClient is an HTTP client for the PIP attributes endpoint.
type pipClient struct {
	baseURL string
	http    *http.Client
}

// newPIPClient returns a pipClient pointing at baseURL.
func newPIPClient(baseURL string) *pipClient {
	return &pipClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// GetAttributes queries GET /pip/attributes/{name} and returns the subject's
// cert-level attributes.
//
// Fail-closed contract:
//   - 404 → certLevel="", certValid=false, err=nil
//   - network error → certLevel="", certValid=false, err=nil
//   - 200 → parse and return the attributes
func (p *pipClient) GetAttributes(name string) (subjectAttrs, error) {
	url := fmt.Sprintf("%s/pip/attributes/%s", p.baseURL, name)
	hc := p.http
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := hc.Get(url)
	if err != nil {
		return subjectAttrs{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return subjectAttrs{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return subjectAttrs{}, nil
	}

	var body struct {
		SystemName string `json:"systemName"`
		CertLevel  string `json:"certLevel"`
		Valid       bool   `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return subjectAttrs{}, nil
	}
	return subjectAttrs{
		CertLevel: body.CertLevel,
		CertValid: body.Valid,
	}, nil
}
