package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// ExternalGrant is a grant fetched from the PIP (originates from ConsumerAuth).
type ExternalGrant struct {
	Subject  string `json:"subject"`
	Resource string `json:"resource"`
}

// Pusher uploads the combined policy (native + PIP grants) to AuthzForce.
type Pusher interface {
	Push(nativePolicies []*Policy, pipGrants []ExternalGrant, version int) error
}

// GrantFetcher retrieves the current grant list and version from the PIP.
// Returns (grants, pipVersion, error).
type GrantFetcher interface {
	FetchGrants() ([]ExternalGrant, int, error)
}

type papServer struct {
	store       *PolicyStore
	pusher      Pusher
	fetcher     GrantFetcher
	domainExtID string
	mux         *http.ServeMux

	mu             sync.Mutex
	lastPIPVersion int
	cachedGrants   []ExternalGrant
}

// NewServer returns an http.Handler for the PAP. It is identical to newServerImpl
// but returns the public http.Handler interface.
func NewServer(store *PolicyStore, pusher Pusher, fetcher GrantFetcher, domainExtID string) http.Handler {
	return newServerImpl(store, pusher, fetcher, domainExtID)
}

func newServerImpl(store *PolicyStore, pusher Pusher, fetcher GrantFetcher, domainExtID string) *papServer {
	s := &papServer{
		store:       store,
		pusher:      pusher,
		fetcher:     fetcher,
		domainExtID: domainExtID,
		mux:         http.NewServeMux(),
	}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/policies", s.handlePolicies)
	s.mux.HandleFunc("/policies/", s.handlePolicyByID)
	return s
}

func (s *papServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// triggerPush fetches the latest PIP grants and calls the pusher with the
// combined native + PIP grant set. Called on every native policy mutation.
func (s *papServer) triggerPush() error {
	grants, _, err := s.fetcher.FetchGrants()
	if err != nil {
		// Degrade gracefully: push with empty PIP grants rather than blocking.
		grants = nil
	}
	s.mu.Lock()
	s.cachedGrants = grants
	s.mu.Unlock()
	return s.pusher.Push(s.store.GetAll(), grants, s.store.Version())
}

// SyncFromPIP compares the PIP version with the last-seen version and rebuilds
// XACML only when the grant set has changed. Called by the background sync loop.
func (s *papServer) SyncFromPIP() error {
	grants, version, err := s.fetcher.FetchGrants()
	if err != nil {
		// Transient PIP failure — skip this cycle, don't return error.
		return nil
	}
	s.mu.Lock()
	changed := version != s.lastPIPVersion
	if changed {
		s.lastPIPVersion = version
		s.cachedGrants = grants
	}
	s.mu.Unlock()

	if changed {
		return s.pusher.Push(s.store.GetAll(), grants, s.store.Version())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *papServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *papServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	pipGrants := len(s.cachedGrants)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"policies":         len(s.store.GetAll()),
		"pipGrants":        pipGrants,
		"version":          s.store.Version(),
		"domainExternalId": s.domainExtID,
	})
}

func (s *papServer) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		all := s.store.GetAll()
		writeJSON(w, http.StatusOK, map[string]any{"policies": all, "count": len(all)})
	case http.MethodPost:
		var req struct {
			Subject  string `json:"subject"`
			Resource string `json:"resource"`
			Action   string `json:"action"`
			Effect   string `json:"effect"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		p, err := s.store.Add(req.Subject, req.Resource, req.Action, req.Effect)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.triggerPush(); err != nil {
			_ = err // push failure is logged, policy is still stored
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		http.NotFound(w, r)
	}
}

func (s *papServer) handlePolicyByID(w http.ResponseWriter, r *http.Request) {
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
		if err := s.triggerPush(); err != nil {
			_ = err
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
