// Package api implements the HTTP interface for the Blacklist core system.
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"arrowhead/core/internal/blacklist/model"
	"arrowhead/core/internal/blacklist/service"
	"arrowhead/core/internal/httputil"
)

const blOrigin = "blacklist"

type Handler struct {
	svc         *service.BlacklistService
	authURL     string // optional: Authentication URL for discovery Bearer enforcement
	mgmtAuthURL string // optional: Authentication URL for management access control
}

// NewHandler creates a Blacklist HTTP handler.
// authURL is the base URL of the Authentication system for discovery token enforcement (BLACKLIST_AUTH_URL).
// mgmtAuthURL is the Authentication URL for management access control (MGMT_AUTH_URL).
// Pass "" to disable the respective check.
func NewHandler(svc *service.BlacklistService, authURL, mgmtAuthURL string) http.Handler {
	h := &Handler{svc: svc, authURL: authURL, mgmtAuthURL: mgmtAuthURL}
	mux := http.NewServeMux()

	// Discovery
	mux.HandleFunc("GET /blacklist/lookup", h.handleLookup)
	mux.HandleFunc("GET /blacklist/check/{systemName}", h.handleCheck)

	// Management
	mux.HandleFunc("POST /blacklist/mgmt/query", h.handleMgmtQuery)
	mux.HandleFunc("POST /blacklist/mgmt/create", h.handleMgmtCreate)
	mux.HandleFunc("DELETE /blacklist/mgmt/remove", h.handleMgmtRemove)

	// Health
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("GET /blacklist/health", h.handleHealth)

	return mux
}

// requireBearer checks for a Bearer token when authURL is configured.
// Returns false (and writes 401) if token is missing.
func (h *Handler) requireBearer(w http.ResponseWriter, r *http.Request) bool {
	if h.authURL == "" {
		return true
	}
	token := httputil.ExtractBearer(r)
	if token == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Authorization: Bearer token required", blOrigin)
		return false
	}
	return true
}

// GET /blacklist/lookup
// Returns all active, non-expired entries applicable to the calling system.
func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	if !h.requireBearer(w, r) {
		return
	}
	entries := activeEntries(h.svc.Query(nil))
	httputil.WriteJSON(w, http.StatusOK, entriesResponse(entries), blOrigin)
}

// GET /blacklist/check/{systemName}
func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !h.requireBearer(w, r) {
		return
	}
	systemName := r.PathValue("systemName")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.svc.IsBlacklisted(systemName)) //nolint:errcheck
}

// POST /blacklist/mgmt/query
func (h *Handler) handleMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, blOrigin) {
		return
	}
	var body struct {
		SystemNames []string `json:"systemNames"`
		Active      *bool    `json:"active"`
		Mode        string   `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", blOrigin)
		return
	}
	// Handle mode parameter (ACTIVES, INACTIVES, ALL)
	if body.Mode != "" {
		switch body.Mode {
		case "ACTIVES":
			t := true
			body.Active = &t
		case "INACTIVES":
			f := false
			body.Active = &f
		case "ALL":
			// leave Active nil
		default:
			httputil.WriteError(w, http.StatusBadRequest, "invalid mode: must be ACTIVES, INACTIVES, or ALL", blOrigin)
			return
		}
	}
	var filter *service.QueryFilter
	if body.SystemNames != nil || body.Active != nil {
		filter = &service.QueryFilter{SystemNames: body.SystemNames, Active: body.Active}
	}
	entries := h.svc.Query(filter)
	httputil.WriteJSON(w, http.StatusOK, entriesResponse(entries), blOrigin)
}

// POST /blacklist/mgmt/create
func (h *Handler) handleMgmtCreate(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, blOrigin) {
		return
	}
	var body struct {
		Entries []struct {
			SystemName string `json:"systemName"`
			Reason     string `json:"reason"`
			ExpiresAt  string `json:"expiresAt"`
			CreatedBy  string `json:"createdBy"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", blOrigin)
		return
	}
	// Validate all entries before creating any.
	for _, e := range body.Entries {
		if strings.TrimSpace(e.Reason) == "" {
			httputil.WriteError(w, http.StatusBadRequest, "reason is required for all entries", blOrigin)
			return
		}
	}
	created := make([]model.Entry, 0, len(body.Entries))
	for _, e := range body.Entries {
		var expiresAt time.Time
		if e.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, e.ExpiresAt)
			if err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "invalid expiresAt format; use RFC3339", blOrigin)
				return
			}
			expiresAt = t
		}
		entry := h.svc.Add(e.SystemName, e.Reason, expiresAt, e.CreatedBy)
		created = append(created, entry)
	}
	httputil.WriteJSON(w, http.StatusCreated, entriesResponse(created), blOrigin)
}

// DELETE /blacklist/mgmt/remove?names=name1,name2
func (h *Handler) handleMgmtRemove(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, blOrigin) {
		return
	}
	namesParam := r.URL.Query().Get("names")
	count := 0
	for _, name := range strings.Split(namesParam, ",") {
		name = strings.TrimSpace(name)
		if name != "" && h.svc.Remove(name) {
			count++
		}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"count": count}, blOrigin)
}

// GET /health, GET /blacklist/health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "blacklist"}, blOrigin)
}

// ---- helpers ----

type entryDTO struct {
	SystemName string  `json:"systemName"`
	Reason     string  `json:"reason"`
	ExpiresAt  *string `json:"expiresAt,omitempty"`
	Active     bool    `json:"active"`
	CreatedBy  string  `json:"createdBy"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

func toDTO(e model.Entry) entryDTO {
	d := entryDTO{
		SystemName: e.SystemName,
		Reason:     e.Reason,
		Active:     e.Active,
		CreatedBy:  e.CreatedBy,
		CreatedAt:  e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  e.UpdatedAt.Format(time.RFC3339),
	}
	if !e.ExpiresAt.IsZero() {
		s := e.ExpiresAt.Format(time.RFC3339)
		d.ExpiresAt = &s
	}
	return d
}

func entriesResponse(entries []model.Entry) map[string]any {
	dtos := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, toDTO(e))
	}
	return map[string]any{"entries": dtos, "count": len(dtos)}
}

func activeEntries(entries []model.Entry) []model.Entry {
	var out []model.Entry
	now := time.Now()
	for _, e := range entries {
		if !e.Active {
			continue
		}
		if !e.ExpiresAt.IsZero() && e.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, e)
	}
	return out
}
