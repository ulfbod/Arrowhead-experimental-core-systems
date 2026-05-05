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
//   AUTHZFORCE_URL    AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//   AUTHZFORCE_DOMAIN AuthzForce domain externalId (default: arrowhead-exp5)
//   CONSUMERAUTH_URL  ConsumerAuthorization base URL (default: http://consumerauth:8082)
//   SYNC_INTERVAL     Sync period (default: 30s)
//   PORT              HTTP health/status port (default: 9095)
//   AUTH_URL          Authentication URL for Bearer token (optional)
//   SYSTEM_NAME       System name for authentication (default: policy-sync)
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
	azURL        := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt  := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp5")
	caURL        := envOr("CONSUMERAUTH_URL", "http://consumerauth:8082")
	port         := envOr("PORT", "9095")
	authURL      := os.Getenv("AUTH_URL")

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

	// currentIntervalNs holds the sync interval in nanoseconds and can be
	// updated at runtime via POST /config.  atomic.Int64 is safe without a mutex.
	var currentIntervalNs atomic.Int64
	currentIntervalNs.Store(syncInterval.Nanoseconds())

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
			"synced":          synced.Load(),
			"version":         s.version,
			"domainId":        s.domainID,
			"domainExternalId": s.domainExtID,
			"grants":          s.grantsCount,
			"lastSyncedAt":    lastAt,
			"syncInterval":    time.Duration(currentIntervalNs.Load()).String(),
		})
	})

	// /config allows changing SYNC_INTERVAL at runtime (GET or POST).
	// POST body: {"syncInterval":"15s"}.  Minimum 1s.
	// Note: the change takes effect at the start of the next sleep, so the
	// current sleep is not interrupted.
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				SyncInterval string `json:"syncInterval"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			d, err := time.ParseDuration(req.SyncInterval)
			if err != nil || d < time.Second {
				http.Error(w, "syncInterval must be a valid duration ≥ 1s", http.StatusBadRequest)
				return
			}
			currentIntervalNs.Store(d.Nanoseconds())
			log.Printf("[policy-sync] SYNC_INTERVAL updated to %s", d)
		case http.MethodGet:
			// fall through to response
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"syncInterval": time.Duration(currentIntervalNs.Load()).String(),
		})
	})

	go func() {
		log.Printf("[policy-sync] HTTP server on :%s", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatal(err)
		}
	}()

	// Initialize: create domain + first sync.
	log.Printf("[policy-sync] initializing domain %q in AuthzForce (%s)", azDomainExt, azURL)
	for attempt := 1; ; attempt++ {
		if err := s.init(azDomainExt); err != nil {
			log.Printf("[policy-sync] init attempt %d failed: %v — retrying in 5s", attempt, err)
			time.Sleep(5 * time.Second)
			continue
		}
		synced.Store(true)
		log.Printf("[policy-sync] initialized domain=%s version=%d", s.domainID, s.version)
		break
	}

	// Periodic sync loop.  Uses currentIntervalNs so that /config changes
	// take effect on the next iteration without restarting the process.
	for {
		time.Sleep(time.Duration(currentIntervalNs.Load()))
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
