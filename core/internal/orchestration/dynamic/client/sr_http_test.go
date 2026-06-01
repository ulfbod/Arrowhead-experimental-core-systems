package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/orchestration/dynamic/client"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

// ah5Response returns a handler serving a minimal AH5 service-discovery/lookup response.
func ah5Response(providerNames ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type prov struct {
			Name string `json:"name"`
		}
		type iface struct {
			TemplateName string `json:"templateName"`
		}
		type inst struct {
			InstanceID            string  `json:"instanceId"`
			Provider              prov    `json:"provider"`
			ServiceDefinitionName string  `json:"serviceDefinitionName"`
			Interfaces            []iface `json:"interfaces"`
		}
		type resp struct {
			Entries []inst `json:"entries"`
			Count   int    `json:"count"`
		}
		var entries []inst
		for _, p := range providerNames {
			entries = append(entries, inst{
				InstanceID:            p + "|svc|1.0.0",
				Provider:              prov{Name: p},
				ServiceDefinitionName: "svc",
				Interfaces:            []iface{{TemplateName: "HTTP"}},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{Entries: entries, Count: len(entries)}) //nolint:errcheck
	}
}

// legacyResponse returns a handler serving a minimal /serviceregistry/query response.
func legacyResponse(providerNames ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type sys struct {
			SystemName string `json:"systemName"`
			Address    string `json:"address"`
			Port       int    `json:"port"`
		}
		type inst struct {
			ServiceDefinition string   `json:"serviceDefinition"`
			ProviderSystem    sys      `json:"providerSystem"`
			ServiceUri        string   `json:"serviceUri"`
			Interfaces        []string `json:"interfaces"`
		}
		type resp struct {
			ServiceQueryData []inst `json:"serviceQueryData"`
			UnfilteredHits   int    `json:"unfilteredHits"`
		}
		var entries []inst
		for _, p := range providerNames {
			entries = append(entries, inst{
				ServiceDefinition: "svc",
				ProviderSystem:    sys{SystemName: p, Address: "127.0.0.1", Port: 9000},
				ServiceUri:        "/svc",
				Interfaces:        []string{"HTTP-INSECURE-JSON"},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{ServiceQueryData: entries, UnfilteredHits: len(entries)}) //nolint:errcheck
	}
}

// muxServer routes /serviceregistry/service-discovery/lookup and /serviceregistry/query
// to different handlers.
func muxServer(ah5 http.HandlerFunc, legacy http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/serviceregistry/service-discovery/lookup", ah5)
	mux.HandleFunc("/serviceregistry/query", legacy)
	return httptest.NewServer(mux)
}

func baseReq(svcDef string) orchmodel.OrchestrationRequest {
	return orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer"},
		RequestedService: orchmodel.ServiceRequirement{ServiceDefinition: svcDef},
	}
}

// ─── G58 — SR bridge tests ────────────────────────────────────────────────────

// TestSRHTTPClientBridgesLegacyStore — when AH5 store is empty, legacy results are returned.
func TestSRHTTPClientBridgesLegacyStore(t *testing.T) {
	srv := muxServer(
		ah5Response(),              // AH5 returns nothing
		legacyResponse("legacy-p"), // legacy has one result
	)
	defer srv.Close()

	c := client.NewSRHTTPClient(srv.URL, http.DefaultClient)
	results, err := c.LookupServices(context.Background(), baseReq("svc"))
	if err != nil {
		t.Fatalf("LookupServices: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (from legacy), got %d", len(results))
	}
	if results[0].ProviderName != "legacy-p" {
		t.Errorf("expected legacy-p, got %q", results[0].ProviderName)
	}
}

// TestSRHTTPClientAH5TakesPriority — when the same provider is in both stores, AH5 wins.
func TestSRHTTPClientAH5TakesPriority(t *testing.T) {
	srv := muxServer(
		ah5Response("shared-p"),   // AH5 has the provider
		legacyResponse("shared-p"), // legacy has the same provider
	)
	defer srv.Close()

	c := client.NewSRHTTPClient(srv.URL, http.DefaultClient)
	results, err := c.LookupServices(context.Background(), baseReq("svc"))
	if err != nil {
		t.Fatalf("LookupServices: %v", err)
	}
	// Should return exactly one result (no duplicate).
	if len(results) != 1 {
		t.Fatalf("expected 1 result (AH5 deduplicates legacy), got %d", len(results))
	}
	// AH5 result has InstanceID set; legacy does not.
	if results[0].ServiceInstanceId == "" {
		t.Error("expected AH5 result (has ServiceInstanceId), got legacy result")
	}
}

// TestSRHTTPClientMergesBothStores — disjoint providers from both stores are merged.
func TestSRHTTPClientMergesBothStores(t *testing.T) {
	srv := muxServer(
		ah5Response("ah5-p"),    // AH5 provider
		legacyResponse("leg-p"), // different legacy provider
	)
	defer srv.Close()

	c := client.NewSRHTTPClient(srv.URL, http.DefaultClient)
	results, err := c.LookupServices(context.Background(), baseReq("svc"))
	if err != nil {
		t.Fatalf("LookupServices: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (one from each store), got %d", len(results))
	}
	names := map[string]bool{results[0].ProviderName: true, results[1].ProviderName: true}
	if !names["ah5-p"] || !names["leg-p"] {
		t.Errorf("expected ah5-p and leg-p, got %v", names)
	}
}

// TestSRHTTPClientLegacyFailOpen — if legacy endpoint is unreachable, AH5 results still returned.
func TestSRHTTPClientLegacyFailOpen(t *testing.T) {
	// AH5 server with a working AH5 endpoint; no legacy endpoint.
	ah5srv := httptest.NewServer(http.HandlerFunc(ah5Response("ah5-p")))
	defer ah5srv.Close()

	c := client.NewSRHTTPClient(ah5srv.URL, http.DefaultClient)
	results, err := c.LookupServices(context.Background(), baseReq("svc"))
	if err != nil {
		t.Fatalf("LookupServices: %v", err)
	}
	if len(results) != 1 || results[0].ProviderName != "ah5-p" {
		t.Errorf("expected 1 AH5 result, got %v", results)
	}
}

// ─── G55 — versionRequirement forwarded to SR ────────────────────────────────

// TestSRHTTPClientForwardsVersionRequirement — versionRequirement is sent in the lookup body.
func TestSRHTTPClientForwardsVersionRequirement(t *testing.T) {
	var capturedBody map[string]any
	srv := muxServer(
		func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"entries": []any{}, "count": 0}) //nolint:errcheck
		},
		legacyResponse(),
	)
	defer srv.Close()

	c := client.NewSRHTTPClient(srv.URL, http.DefaultClient)
	req := baseReq("svc")
	req.RequestedService.VersionRequirement = "2.0.0"
	c.LookupServices(context.Background(), req) //nolint:errcheck

	versions, ok := capturedBody["versions"].([]any)
	if !ok || len(versions) == 0 {
		t.Fatalf("expected versions field in SR lookup body, got %v", capturedBody)
	}
	if versions[0] != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %v", versions[0])
	}
}
