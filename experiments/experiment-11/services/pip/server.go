package main

import (
	"encoding/json"
	"net/http"
)

type pipServer struct {
	store          *GrantStore
	consumerAuthURL string
	mux            *http.ServeMux
}

// NewServer returns an http.Handler with all PIP routes registered.
func NewServer(store *GrantStore, consumerAuthURL string) http.Handler {
	s := &pipServer{
		store:           store,
		consumerAuthURL: consumerAuthURL,
		mux:             http.NewServeMux(),
	}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/grants", s.handleGrants)
	return s
}

func (s *pipServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// GET /health
func (s *pipServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /status
func (s *pipServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	lastSync := ""
	if t := s.store.LastSyncAt(); !t.IsZero() {
		lastSync = t.Format("2006-01-02T15:04:05Z")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"synced":          s.store.Synced(),
		"version":         s.store.Version(),
		"grants":          len(s.store.GetAll()),
		"lastSyncAt":      lastSync,
		"consumerAuthURL": s.consumerAuthURL,
	})
}

// GET /grants[?subject=X][?resource=Y]
func (s *pipServer) handleGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	subject := r.URL.Query().Get("subject")
	resource := r.URL.Query().Get("resource")

	// Specific subject+resource lookup
	if subject != "" && resource != "" {
		granted := s.store.IsGranted(subject, resource)
		writeJSON(w, http.StatusOK, map[string]any{
			"subject":  subject,
			"resource": resource,
			"granted":  granted,
			"count":    boolToInt(granted),
		})
		return
	}

	// Subject filter
	var grants []Grant
	if subject != "" {
		grants = s.store.GetBySubject(subject)
	} else {
		grants = s.store.GetAll()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"grants":  grants,
		"count":   len(grants),
		"version": s.store.Version(),
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
