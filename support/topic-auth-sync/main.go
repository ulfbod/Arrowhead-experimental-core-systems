package main

import (
	"encoding/json"
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
