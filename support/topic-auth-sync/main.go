package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	gosync "sync"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := config{
		consumerAuthURL:  envOr("CONSUMERAUTH_URL", "http://localhost:8082"),
		rmqBase:          envOr("RABBITMQ_URL", "http://localhost:15672"),
		rmqAdminUser:     envOr("RABBITMQ_ADMIN_USER", "guest"),
		rmqAdminPass:     envOr("RABBITMQ_ADMIN_PASS", "guest"),
		rmqVhost:         envOr("RABBITMQ_VHOST", "/"),
		rmqExchange:      envOr("RABBITMQ_EXCHANGE", "arrowhead"),
		consumerPassword: envOr("CONSUMER_PASSWORD", "consumer-secret"),
		publisherUser:    envOr("PUBLISHER_USER", "robot-fleet"),
		publisherPass:    envOr("PUBLISHER_PASSWORD", "fleet-secret"),
	}

	intervalStr := envOr("SYNC_INTERVAL", "10s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Fatalf("invalid SYNC_INTERVAL %q: %v", intervalStr, err)
	}

	port := envOr("PORT", "9090")

	s := newSyncer(cfg)

	// Phase 2: Authenticate if AUTH_URL is configured.
	// Fail-soft: log and proceed without token if auth is unreachable.
	// ConsumerAuth does not currently enforce tokens, so this future-proofs the calls.
	if authURL := envOr("AUTH_URL", ""); authURL != "" {
		type loginReq struct {
			SystemName  string `json:"systemName"`
			Credentials string `json:"credentials"`
		}
		type loginResp struct {
			Token string `json:"token"`
		}
		sysName  := envOr("SYSTEM_NAME", "topic-auth-sync")
		sysCreds := envOr("SYSTEM_CREDENTIALS", "sync-secret")
		body, _ := json.Marshal(loginReq{SystemName: sysName, Credentials: sysCreds})
		resp, err := http.Post(authURL+"/authentication/identity/login", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[main] auth login failed: %v — proceeding without token", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusCreated {
				var lr loginResp
				if decErr := json.NewDecoder(resp.Body).Decode(&lr); decErr == nil && lr.Token != "" {
					s.setToken(lr.Token)
					log.Printf("[main] authenticated as %q", sysName)
				}
			} else {
				b, _ := io.ReadAll(resp.Body)
				log.Printf("[main] auth login returned %d: %s — proceeding without token", resp.StatusCode, string(b))
			}
		}
	}

	var mu gosync.Mutex
	ready := false

	// Health endpoint — returns 200 once the first sync has succeeded.
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isReady := ready
		mu.Unlock()

		if !isReady {
			http.Error(w, `{"status":"starting"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	go func() {
		log.Printf("[main] health server listening on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("health server: %v", err)
		}
	}()

	// Sync loop.
	for {
		if err := s.sync(); err != nil {
			log.Printf("[main] sync error: %v", err)
		} else {
			mu.Lock()
			ready = true
			mu.Unlock()
			log.Printf("[main] sync completed successfully")
		}
		time.Sleep(interval)
	}
}
