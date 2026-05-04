// policy-sync — projects ConsumerAuthorization grants into AuthzForce XACML policies.
//
// On startup it creates (or looks up) an AuthzForce domain, compiles the
// current CA grant set into a XACML 3.0 PolicySet, and pushes it to the
// AuthzForce PAP.  A background loop re-syncs every SYNC_INTERVAL.
//
// The /health endpoint returns 200 only after the first successful sync,
// making it safe to use as a Docker healthcheck dependency for downstream
// enforcement adapters (topic-auth-xacml, kafka-authz).
//
// Environment variables:
//   AUTHZFORCE_URL   AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//   CONSUMERAUTH_URL ConsumerAuthorization base URL (default: http://consumerauth:8082)
//   SYNC_INTERVAL    Sync period (default: 30s)
//   PORT             HTTP health/status port (default: 9095)
//   AUTH_URL         Authentication URL for Bearer token (optional)
//   SYSTEM_NAME      System name for authentication (default: policy-sync)
//   SYSTEM_CREDENTIALS Credentials for authentication (default: sync-secret)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	az "arrowhead/authzforce"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	azURL   := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	caURL   := envOr("CONSUMERAUTH_URL", "http://consumerauth:8082")
	port    := envOr("PORT", "9095")
	authURL := os.Getenv("AUTH_URL")

	syncInterval, err := time.ParseDuration(envOr("SYNC_INTERVAL", "30s"))
	if err != nil {
		log.Fatalf("invalid SYNC_INTERVAL: %v", err)
	}

	client := az.New(azURL)
	s := newSyncer(client, caURL)

	// Optional Bearer token for ConsumerAuth.
	if authURL != "" {
		tok, err := ahcLogin(authURL,
			envOr("SYSTEM_NAME", "policy-sync"),
			envOr("SYSTEM_CREDENTIALS", "sync-secret"),
		)
		if err != nil {
			log.Printf("[policy-sync] auth login failed: %v — proceeding without token", err)
		} else if tok != "" {
			s.setToken(tok)
			log.Printf("[policy-sync] authenticated")
		}
	}

	// Track whether the first sync has completed.
	var synced atomic.Bool

	// Health/status endpoints.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !synced.Load() {
			http.Error(w, `{"status":"syncing"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastAt := ""
		if !s.lastSyncedAt.IsZero() {
			lastAt = s.lastSyncedAt.UTC().Format(time.RFC3339)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"synced":       synced.Load(),
			"version":      s.version,
			"domainId":     s.domainID,
			"grants":       s.grantsCount,
			"lastSyncedAt": lastAt,
		})
	})

	go func() {
		log.Printf("[policy-sync] HTTP server on :%s", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatal(err)
		}
	}()

	// Initialize: create domain + first sync.
	log.Printf("[policy-sync] initializing domain in AuthzForce (%s)", azURL)
	for attempt := 1; ; attempt++ {
		if err := s.init(); err != nil {
			log.Printf("[policy-sync] init attempt %d failed: %v — retrying in 5s", attempt, err)
			time.Sleep(5 * time.Second)
			continue
		}
		synced.Store(true)
		log.Printf("[policy-sync] initialized domain=%s version=%d", s.domainID, s.version)
		break
	}

	// Periodic sync loop.
	for {
		time.Sleep(syncInterval)
		if err := s.sync(); err != nil {
			log.Printf("[policy-sync] sync error: %v", err)
		} else {
			log.Printf("[policy-sync] sync OK — version=%d", s.version)
		}
	}
}

// ahcLogin authenticates with the Arrowhead Authentication system.
func ahcLogin(authURL, systemName, credentials string) (string, error) {
	type req struct {
		SystemName  string `json:"systemName"`
		Credentials string `json:"credentials"`
	}
	type resp struct {
		Token string `json:"token"`
	}
	body, _ := json.Marshal(req{SystemName: systemName, Credentials: credentials})
	r, err := http.Post(authURL+"/authentication/identity/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(r.Body)
		return "", fmt.Errorf("auth login returned %d: %s", r.StatusCode, b)
	}
	var lr resp
	if err := json.NewDecoder(r.Body).Decode(&lr); err != nil {
		return "", err
	}
	return lr.Token, nil
}
