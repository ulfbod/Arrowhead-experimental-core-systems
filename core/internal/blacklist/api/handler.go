// Package api implements the HTTP interface for the Blacklist core system.
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"arrowhead/core/internal/blacklist/repository"
	"arrowhead/core/internal/blacklist/service"
)

type Handler struct {
	svc *service.BlacklistService
}

func NewHandler(svc *service.BlacklistService) http.Handler {
	h := &Handler{svc: svc}
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

// GET /blacklist/lookup
// Returns all active, non-expired entries applicable to the calling system.
// (Simplified: returns all active entries since auth is not enforced.)
func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	entries := activeEntries(h.svc.Query(nil))
	writeJSON(w, http.StatusOK, entriesResponse(entries))
}

// GET /blacklist/check/{systemName}
func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	systemName := r.PathValue("systemName")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.svc.IsBlacklisted(systemName))
}

// POST /blacklist/mgmt/query
func (h *Handler) handleMgmtQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SystemNames []string `json:"systemNames"`
		Active      *bool    `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	var filter *service.QueryFilter
	if body.SystemNames != nil || body.Active != nil {
		filter = &service.QueryFilter{SystemNames: body.SystemNames, Active: body.Active}
	}
	entries := h.svc.Query(filter)
	writeJSON(w, http.StatusOK, entriesResponse(entries))
}

// POST /blacklist/mgmt/create
func (h *Handler) handleMgmtCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Entries []struct {
			SystemName string `json:"systemName"`
			Reason     string `json:"reason"`
			ExpiresAt  string `json:"expiresAt"`
			CreatedBy  string `json:"createdBy"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Validate all entries before creating any.
	for _, e := range body.Entries {
		if strings.TrimSpace(e.Reason) == "" {
			writeError(w, http.StatusBadRequest, "reason is required for all entries")
			return
		}
	}
	created := make([]repository.Entry, 0, len(body.Entries))
	for _, e := range body.Entries {
		var expiresAt time.Time
		if e.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, e.ExpiresAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid expiresAt format; use RFC3339")
				return
			}
			expiresAt = t
		}
		entry := h.svc.Add(e.SystemName, e.Reason, expiresAt, e.CreatedBy)
		created = append(created, entry)
	}
	writeJSON(w, http.StatusCreated, entriesResponse(created))
}

// DELETE /blacklist/mgmt/remove?names=name1,name2
func (h *Handler) handleMgmtRemove(w http.ResponseWriter, r *http.Request) {
	namesParam := r.URL.Query().Get("names")
	count := 0
	for _, name := range strings.Split(namesParam, ",") {
		name = strings.TrimSpace(name)
		if name != "" && h.svc.Remove(name) {
			count++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// GET /health, GET /blacklist/health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "blacklist"})
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

func toDTO(e repository.Entry) entryDTO {
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

func entriesResponse(entries []repository.Entry) map[string]any {
	dtos := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, toDTO(e))
	}
	return map[string]any{"entries": dtos, "count": len(dtos)}
}

func activeEntries(entries []repository.Entry) []repository.Entry {
	var out []repository.Entry
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	exType := "INVALID_PARAMETER"
	if status == http.StatusNotFound {
		exType = "NOT_FOUND"
	}
	writeJSON(w, status, map[string]any{
		"errorMessage":  msg,
		"errorCode":     status,
		"exceptionType": exType,
		"origin":        "blacklist",
	})
}
