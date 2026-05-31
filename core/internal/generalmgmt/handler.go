package generalmgmt

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"arrowhead/core/internal/httputil"
)

const gmOrigin = "generalmgmt"

// NewHandler returns an http.Handler that serves:
//
//	POST /<prefix>/general/mgmt/logs      — filtered log query
//	GET  /<prefix>/general/mgmt/get-config — config key lookup
//
// The prefix parameter must not start or end with a slash
// (e.g. "serviceregistry", "ca", "serviceorchestration/orchestration").
// config is a map of key→value exposed via the get-config endpoint.
func NewHandler(buf *LogBuffer, prefix string, config map[string]string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /"+prefix+"/general/mgmt/logs", makeLogsHandler(buf))
	mux.HandleFunc("GET /"+prefix+"/general/mgmt/get-config", makeConfigHandler(config))
	return mux
}

// ---- POST /<prefix>/general/mgmt/logs ----------------------------------------

type logsRequest struct {
	Pagination *struct {
		PageNumber int `json:"pageNumber"`
		PageSize   int `json:"pageSize"`
	} `json:"pagination"`
	From      string `json:"from"`
	To        string `json:"to"`
	Severity  string `json:"severity"`
	LoggerStr string `json:"loggerStr"`
}

type logEntryDTO struct {
	LogID     string `json:"logId"`
	EntryDate string `json:"entryDate"`
	Logger    string `json:"logger"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Exception string `json:"exception,omitempty"`
}

func makeLogsHandler(buf *LogBuffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req logsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", gmOrigin)
			return
		}

		// Validate time range.
		if req.From != "" && req.To != "" {
			from, err1 := time.Parse(time.RFC3339, req.From)
			to, err2 := time.Parse(time.RFC3339, req.To)
			if err1 == nil && err2 == nil && from.After(to) {
				httputil.WriteError(w, http.StatusBadRequest, "from must not be after to", gmOrigin)
				return
			}
		}

		entries := buf.Filter(LogFilter{
			From:      req.From,
			To:        req.To,
			Severity:  req.Severity,
			LoggerStr: req.LoggerStr,
		})

		// Pagination.
		pageSize := 20
		pageNumber := 0
		if req.Pagination != nil {
			if req.Pagination.PageSize > 0 {
				pageSize = req.Pagination.PageSize
			}
			if req.Pagination.PageNumber > 0 {
				pageNumber = req.Pagination.PageNumber
			}
		}
		start := pageNumber * pageSize
		if start > len(entries) {
			start = len(entries)
		}
		end := start + pageSize
		if end > len(entries) {
			end = len(entries)
		}
		page := entries[start:end]

		dtos := make([]logEntryDTO, 0, len(page))
		for _, e := range page {
			dtos = append(dtos, logEntryDTO{
				LogID:     e.LogID,
				EntryDate: e.EntryDate.Format(time.RFC3339),
				Logger:    e.Logger,
				Severity:  e.Severity,
				Message:   e.Message,
				Exception: e.Exception,
			})
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"entries": dtos, "count": len(dtos)}, gmOrigin)
	}
}

// ---- GET /<prefix>/general/mgmt/get-config -----------------------------------

func makeConfigHandler(config map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keysParam := r.URL.Query().Get("keys")
		result := make(map[string]string)
		for _, key := range strings.Split(keysParam, ",") {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if val, ok := config[key]; ok {
				result[key] = val
			}
		}
		httputil.WriteJSON(w, http.StatusOK, result, gmOrigin)
	}
}
