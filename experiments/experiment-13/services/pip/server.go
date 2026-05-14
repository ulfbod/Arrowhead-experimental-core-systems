package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type pipServer struct {
	store *SubjectStore
	mux   *http.ServeMux
}

// NewServer builds and returns an http.Handler with all PIP routes registered.
func NewServer(store *SubjectStore) http.Handler {
	s := &pipServer{store: store, mux: http.NewServeMux()}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/subjects", s.handleSubjects)
	s.mux.HandleFunc("/subjects/", s.handleSubjectByName)
	s.mux.HandleFunc("/attributes/", s.handleAttributes)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"subjects": len(s.store.GetAll()),
	})
}

// /subjects — GET list, POST register
func (s *pipServer) handleSubjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		all := s.store.GetAll()
		writeJSON(w, http.StatusOK, map[string]any{"subjects": all, "count": len(all)})
	case http.MethodPost:
		var req struct {
			Name      string `json:"name"`
			CertLevel string `json:"certLevel"`
			Valid     bool   `json:"valid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sub, err := s.store.Register(req.Name, req.CertLevel, req.Valid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, sub)
	default:
		http.NotFound(w, r)
	}
}

// /subjects/{name} — GET, DELETE
func (s *pipServer) handleSubjectByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/subjects/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		sub, ok := s.store.Get(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, sub)
	case http.MethodDelete:
		if !s.store.Delete(name) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// GET /attributes/{name} — returns XACML-ready attributes for a subject
func (s *pipServer) handleAttributes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/attributes/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	sub, ok := s.store.Get(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"systemName": sub.Name,
		"certLevel":  sub.CertLevel,
		"valid":      sub.Valid,
	})
}
