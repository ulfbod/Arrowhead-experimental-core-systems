package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Pusher pushes the current policy set to AuthzForce.
type Pusher interface {
	Push(policies []*Policy, version int) error
}

type server struct {
	store       *PolicyStore
	pusher      Pusher
	domainExtID string
	mux         *http.ServeMux
}

// NewServer builds and returns an http.Handler with all PAP routes registered.
func NewServer(store *PolicyStore, pusher Pusher, domainExtID string) http.Handler {
	s := &server{
		store:       store,
		pusher:      pusher,
		domainExtID: domainExtID,
		mux:         http.NewServeMux(),
	}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/policies", s.handlePolicies)
	s.mux.HandleFunc("/policies/", s.handlePolicyByID)
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// writeJSON encodes v as JSON and sets Content-Type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// handleHealth GET /health
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleStatus GET /status
func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policies":         len(s.store.GetAll()),
		"version":          s.store.Version(),
		"domainExternalId": s.domainExtID,
	})
}

// handlePolicies dispatches GET /policies and POST /policies.
func (s *server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPolicies(w, r)
	case http.MethodPost:
		s.createPolicy(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *server) listPolicies(w http.ResponseWriter, r *http.Request) {
	all := s.store.GetAll()
	writeJSON(w, http.StatusOK, map[string]any{
		"policies": all,
		"count":    len(all),
	})
}

func (s *server) createPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject  string `json:"subject"`
		Resource string `json:"resource"`
		Provider string `json:"provider"` // optional: provider system name for per-provider policies
		Action   string `json:"action"`
		Effect   string `json:"effect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p, err := s.store.Add(req.Subject, req.Resource, req.Provider, req.Action, req.Effect)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.pusher.Push(s.store.GetAll(), s.store.Version()); err != nil {
		// Log but don't fail the request — policy is stored even if push fails.
		_ = err
	}
	writeJSON(w, http.StatusCreated, p)
}

// handlePolicyByID dispatches GET /policies/{id} and DELETE /policies/{id}.
func (s *server) handlePolicyByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/policies/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		p, ok := s.store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodDelete:
		if !s.store.Delete(id) {
			http.NotFound(w, r)
			return
		}
		if err := s.pusher.Push(s.store.GetAll(), s.store.Version()); err != nil {
			_ = err
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
