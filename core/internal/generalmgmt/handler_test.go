package generalmgmt_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/generalmgmt"
)

// newMgmtHandler creates a handler for the given system prefix.
// NewHandler signature: NewHandler(buf *LogBuffer, prefix string, config map[string]string) http.Handler
func newMgmtHandler(prefix string) http.Handler {
	buf := generalmgmt.NewLogBuffer(100)
	return generalmgmt.NewHandler(buf, prefix, map[string]string{"PORT": "8080"})
}

func TestLogEndpointReturns200(t *testing.T) {
	h := newMgmtHandler("serviceregistry")
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/general/mgmt/logs",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0", resp.Count)
	}
}

func TestLogEndpointInvalidTimeRange400(t *testing.T) {
	h := newMgmtHandler("serviceregistry")
	// from > to should return 400.
	body := `{"from":"2025-01-02T00:00:00Z","to":"2025-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/general/mgmt/logs",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for from > to, got %d", w.Code)
	}
}

func TestGetConfigReturnsRequestedKeys(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(10)
	h := generalmgmt.NewHandler(buf, "serviceregistry", map[string]string{
		"PORT":    "8080",
		"DB_PATH": "/data/sr.db",
	})

	req := httptest.NewRequest(http.MethodGet,
		"/serviceregistry/general/mgmt/get-config?keys=PORT,DB_PATH,UNKNOWN", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["PORT"] != "8080" {
		t.Errorf("PORT = %q, want 8080", resp["PORT"])
	}
	if resp["DB_PATH"] != "/data/sr.db" {
		t.Errorf("DB_PATH = %q", resp["DB_PATH"])
	}
	if _, ok := resp["UNKNOWN"]; ok {
		t.Error("unknown key should not appear in response")
	}
}

func TestLogEndpointFiltersEntries(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(100)
	buf.Append(generalmgmt.LogEntry{Message: "warn-msg", Severity: "WARN"})
	buf.Append(generalmgmt.LogEntry{Message: "info-msg", Severity: "INFO"})
	h := generalmgmt.NewHandler(buf, "serviceregistry", nil)

	body := `{"severity":"WARN"}`
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/general/mgmt/logs",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Count   int                      `json:"count"`
		Entries []map[string]interface{} `json:"entries"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1 (WARN only)", resp.Count)
	}
}
