// pip_handlers.go — CA-as-PIP HTTP handlers for experiment-15.
//
// profile-ca now serves PIP API endpoints directly from its cert store,
// removing the separate pip service and the gRPC subscriber.
//
// Design decision D4 (experiment-15):
//   The ProfileCA already holds the authoritative record of every issued,
//   revoked, and expired certificate. Rather than replicating that state into
//   a separate PIP service over gRPC, these handlers expose the cert store
//   directly as PIP endpoints. This eliminates the gRPC fan-out latency, the
//   subscriber service, and the risk of state divergence between CA and PIP.
//
// Endpoints (all under /pip/ prefix, matching PEP client expectations):
//
//	GET /pip/attributes/{cn}  — XACML-ready cert validity attributes
//	GET /pip/subjects         — list all cert records (including revoked)
//	GET /pip/subjects/{cn}    — single cert record detail by CN
//	GET /pip/status           — summary: total subject count
//
// Response format for /pip/attributes/{cn} (must match kafka-authz pip.go
// and Java PipCertValidityChecker):
//
//	{"systemName":"cn","certLevel":"sy","valid":true}
//
// A cert is valid if: record exists AND !record.Revoked AND time.Now().Before(record.ExpiresAt)
package main

import (
	"net/http"
	"strings"
	"time"
)

// pipAttributesResponse is the XACML-ready attribute response format.
// Field names must match what kafka-authz pip.go and Java PipCertValidityChecker expect.
type pipAttributesResponse struct {
	SystemName string `json:"systemName"`
	CertLevel  string `json:"certLevel"`
	Valid       bool   `json:"valid"`
}

// pipSubjectResponse is the per-record detail response for /pip/subjects/{cn}.
type pipSubjectResponse struct {
	CN        string    `json:"cn"`
	OU        string    `json:"ou"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revoked   bool      `json:"revoked"`
	Valid     bool      `json:"valid"`
}

// certIsValid returns true if the record is not revoked and not yet expired.
func certIsValid(rec *CertRecord) bool {
	return !rec.Revoked && time.Now().Before(rec.ExpiresAt)
}

// handlePIPAttributes handles GET /pip/attributes/{cn}.
// Returns XACML-ready cert validity attributes.
// 200 for known certs (valid or revoked/expired), 404 for unknown CN.
func handlePIPAttributes(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}

		// Extract CN from path: /pip/attributes/{cn}
		cn := strings.TrimPrefix(r.URL.Path, "/pip/attributes/")
		if cn == "" {
			writeError(w, http.StatusBadRequest, "cn path parameter required")
			return
		}

		rec, ok := ca.GetRecord(cn)
		if !ok {
			writeError(w, http.StatusNotFound, "certificate not found: "+cn)
			return
		}

		writeJSON(w, http.StatusOK, pipAttributesResponse{
			SystemName: rec.CN,
			CertLevel:  rec.OU,
			Valid:       certIsValid(rec),
		})
	}
}

// handlePIPSubjects handles GET /pip/subjects.
// Returns all cert records including revoked.
func handlePIPSubjects(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}

		all := ca.GetAllRecords()
		subjects := make([]pipSubjectResponse, 0, len(all))
		for _, rec := range all {
			subjects = append(subjects, pipSubjectResponse{
				CN:        rec.CN,
				OU:        rec.OU,
				IssuedAt:  rec.IssuedAt,
				ExpiresAt: rec.ExpiresAt,
				Revoked:   rec.Revoked,
				Valid:     certIsValid(rec),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"subjects": subjects,
			"count":    len(subjects),
		})
	}
}

// handlePIPSubject handles GET /pip/subjects/{cn}.
// Returns detail for a single cert record.
func handlePIPSubject(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}

		cn := strings.TrimPrefix(r.URL.Path, "/pip/subjects/")
		if cn == "" {
			writeError(w, http.StatusBadRequest, "cn path parameter required")
			return
		}

		rec, ok := ca.GetRecord(cn)
		if !ok {
			writeError(w, http.StatusNotFound, "certificate not found: "+cn)
			return
		}

		writeJSON(w, http.StatusOK, pipSubjectResponse{
			CN:        rec.CN,
			OU:        rec.OU,
			IssuedAt:  rec.IssuedAt,
			ExpiresAt: rec.ExpiresAt,
			Revoked:   rec.Revoked,
			Valid:     certIsValid(rec),
		})
	}
}

// handlePIPStatus handles GET /pip/status.
// Returns total subject count (all records including revoked).
func handlePIPStatus(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}

		all := ca.GetAllRecords()
		writeJSON(w, http.StatusOK, map[string]any{
			"subjectCount": len(all),
		})
	}
}
