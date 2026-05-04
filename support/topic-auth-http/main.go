package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
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
		rmqBase:          envOr("RABBITMQ_MGMT_URL", ""),
		rmqAdminUser:     envOr("RABBITMQ_ADMIN_USER", "guest"),
		rmqAdminPass:     envOr("RABBITMQ_ADMIN_PASS", "guest"),
		rmqVhost:         envOr("RABBITMQ_VHOST", "/"),
		consumerPassword: envOr("CONSUMER_PASSWORD", "consumer-secret"),
		publisherUser:    envOr("PUBLISHER_USER", "robot-fleet"),
		publisherPass:    envOr("PUBLISHER_PASSWORD", "fleet-secret"),
	}

	cacheTTL, err := time.ParseDuration(envOr("CACHE_TTL", "0s"))
	if err != nil {
		log.Fatalf("invalid CACHE_TTL: %v", err)
	}

	revocationInterval, err := time.ParseDuration(envOr("REVOCATION_INTERVAL", "15s"))
	if err != nil {
		log.Fatalf("invalid REVOCATION_INTERVAL: %v", err)
	}

	port := envOr("PORT", "9090")

	cache := newRulesCache(cacheTTL)

	// Create RMQ management client if URL is configured.
	var rmq *rmqClient
	if cfg.rmqBase != "" {
		rmq = newRMQClient(cfg.rmqBase, cfg.rmqAdminUser, cfg.rmqAdminPass, cfg.rmqVhost)
	}

	s := newSyncer(cfg, rmq)

	// Optional auth login.
	if authURL := os.Getenv("AUTH_URL"); authURL != "" {
		type loginReq struct {
			SystemName  string `json:"systemName"`
			Credentials string `json:"credentials"`
		}
		type loginResp struct {
			Token string `json:"token"`
		}
		sysName := envOr("SYSTEM_NAME", "topic-auth-http")
		sysCreds := envOr("SYSTEM_CREDENTIALS", "sync-secret")
		body, _ := json.Marshal(loginReq{SystemName: sysName, Credentials: sysCreds})
		resp, loginErr := http.Post(authURL+"/authentication/identity/login", "application/json", bytes.NewReader(body))
		if loginErr != nil {
			log.Printf("[main] auth login failed: %v — proceeding without token", loginErr)
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

	fetchRules := func(ctx context.Context) ([]AuthRule, error) {
		if rules, ok := cache.get(); ok {
			return rules, nil
		}
		rules, err := s.fetchRules()
		if err != nil {
			return nil, err
		}
		cache.set(rules)
		return rules, nil
	}

	mux := http.NewServeMux()
	newAuthServer(cfg, fetchRules).register(mux)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen :%s: %v", port, err)
	}
	log.Printf("[main] topic-auth-http listening on :%s (cache_ttl=%s, revocation_interval=%s)",
		port, cacheTTL, revocationInterval)

	// Start revocation loop: periodically close connections for revoked consumers.
	if rmq != nil {
		go func() {
			ticker := time.NewTicker(revocationInterval)
			defer ticker.Stop()
			for range ticker.C {
				if err := s.enforceRevocations(); err != nil {
					log.Printf("[revoke] %v", err)
				}
			}
		}()
		log.Printf("[main] revocation loop active (interval=%s)", revocationInterval)
	} else {
		log.Printf("[main] RABBITMQ_MGMT_URL not set — revocation loop disabled")
	}

	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}
