package api_test

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	qosapi "arrowhead/core/internal/deviceqoseval/api"
	"arrowhead/core/internal/deviceqoseval/model"
	"arrowhead/core/internal/deviceqoseval/repository"
	"arrowhead/core/internal/deviceqoseval/service"
)

func newHandler() http.Handler {
	repo := repository.NewMemoryRepository()
	eval := service.NewEvaluator(repo)
	return qosapi.NewHandler(eval)
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHealthEndpoint(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest(http.MethodGet, "/deviceqosevaluator/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("health: want 200, got %d", w.Code)
	}
}

func TestMeasureEndpointReachable(t *testing.T) {
	// Start a real TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close() //nolint:errcheck
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	h := newHandler()
	w := postJSON(t, h, "/deviceqosevaluator/quality-evaluation/measure", map[string]string{
		"host": "127.0.0.1",
		"port": port,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("measure: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var rec model.QoSRecord
	json.NewDecoder(w.Body).Decode(&rec) //nolint:errcheck
	if !rec.Reachable {
		t.Errorf("reachable = false, want true")
	}
}

func TestMgmtQueryEndpoint(t *testing.T) {
	h := newHandler()
	// Measure something (will be unreachable but creates record)
	postJSON(t, h, "/deviceqosevaluator/quality-evaluation/measure", map[string]string{
		"host": "host-a",
		"port": "1",
	})
	postJSON(t, h, "/deviceqosevaluator/quality-evaluation/measure", map[string]string{
		"host": "host-b",
		"port": "1",
	})

	w := postJSON(t, h, "/deviceqosevaluator/quality-evaluation/mgmt/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("query: want 200, got %d", w.Code)
	}
	var resp model.QueryResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
}

func TestMgmtQueryFilterByHost(t *testing.T) {
	h := newHandler()
	postJSON(t, h, "/deviceqosevaluator/quality-evaluation/measure", map[string]string{"host": "host-a", "port": "1"})
	postJSON(t, h, "/deviceqosevaluator/quality-evaluation/measure", map[string]string{"host": "host-b", "port": "1"})

	w := postJSON(t, h, "/deviceqosevaluator/quality-evaluation/mgmt/query", map[string]string{"host": "host-a"})
	if w.Code != http.StatusOK {
		t.Fatalf("query by host: want 200, got %d", w.Code)
	}
	var resp model.QueryResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if resp.Records[0].Host != "host-a" {
		t.Errorf("host = %q, want host-a", resp.Records[0].Host)
	}
}
