// handlers.go — HTTP handlers for profile-ca.
package main

import (
	"encoding/json"
	"net/http"
	"time"
)

type certResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	Profile     string `json:"profile"`
	IssuedAt    string `json:"issuedAt"`
}

type issueRequest struct {
	SystemName string `json:"systemName"`
}

func handleInfo(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"commonName":  "Arrowhead Local Cloud CA",
			"certificate": ca.CACertPEM(),
		})
	}
}

func handleBootstrapOnboarding(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		certPEM, keyPEM, err := ca.IssueOnboardingCert(req.SystemName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, certResponse{
			SystemName:  req.SystemName,
			Certificate: certPEM,
			PrivateKey:  keyPEM,
			Profile:     string(ProfileOnboarding),
			IssuedAt:    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleIssueInfra(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		certPEM, keyPEM, err := ca.IssueInfraCert(req.SystemName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, certResponse{
			SystemName:  req.SystemName,
			Certificate: certPEM,
			PrivateKey:  keyPEM,
			Profile:     string(ProfileSystem),
			IssuedAt:    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleDeviceCert(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			writeError(w, http.StatusUnauthorized, "onboarding client certificate required")
			return
		}
		clientCert := r.TLS.PeerCertificates[0]

		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		certPEM, keyPEM, err := ca.IssueDeviceCert(req.SystemName, clientCert)
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, certResponse{
			SystemName:  req.SystemName,
			Certificate: certPEM,
			PrivateKey:  keyPEM,
			Profile:     string(ProfileDevice),
			IssuedAt:    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleSystemCert(ca *ProfileCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			writeError(w, http.StatusUnauthorized, "device client certificate required")
			return
		}
		clientCert := r.TLS.PeerCertificates[0]

		var req issueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		certPEM, keyPEM, err := ca.IssueSystemCert(req.SystemName, clientCert)
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, certResponse{
			SystemName:  req.SystemName,
			Certificate: certPEM,
			PrivateKey:  keyPEM,
			Profile:     string(ProfileSystem),
			IssuedAt:    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
