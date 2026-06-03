// server.go — HTTP/HTTPS handlers for portal-cloud-ml.
//
// Plain HTTP endpoints (PORT):
//
//	GET /health   — liveness probe
//	GET /stats    — message counters and status
//
// HTTPS endpoints (TLS_PORT) — served with PKI system cert, consumed by pki-rest-authz:
//
//	GET /health              — liveness probe
//	GET /stats               — message counters
//	GET /telemetry/latest    — latest aggregated telemetry payload
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// makeHTTPHandler returns a mux for the plain HTTP health port.
func makeHTTPHandler(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(store.Stats())
	})
	return mux
}

// makeHTTPSHandler returns a mux for the HTTPS REST port consumed by service partners.
func makeHTTPSHandler(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(store.Stats())
	})
	mux.HandleFunc("/telemetry/latest", func(w http.ResponseWriter, _ *http.Request) {
		payload := store.Latest()
		w.Header().Set("Content-Type", "application/json")
		if payload == nil {
			w.Write([]byte(`{"status":"no data yet"}`))
			return
		}
		w.Write(payload)
	})
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok","service":"portal-cloud-ml"}`)
}
