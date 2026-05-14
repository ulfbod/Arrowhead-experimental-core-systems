package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

type config struct {
	port            string
	consumerAuthURL string
	syncInterval    time.Duration
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func configFromEnv() config {
	intervalStr := envOr("SYNC_INTERVAL", "10s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 10 * time.Second
	}
	return config{
		port:            envOr("PORT", "9406"),
		consumerAuthURL: envOr("CONSUMERAUTH_URL", "http://consumerauth:8082"),
		syncInterval:    interval,
	}
}

func main() {
	cfg := configFromEnv()
	store := NewGrantStore()

	// Initial sync — retry until ConsumerAuth is reachable.
	log.Printf("PIP: connecting to ConsumerAuth at %s", cfg.consumerAuthURL)
	for {
		if err := fetchAndUpdate(cfg.consumerAuthURL, store); err != nil {
			log.Printf("PIP: initial sync failed: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
	log.Printf("PIP: initial sync complete, %d grants", len(store.GetAll()))

	// Background sync loop.
	ctx := context.Background()
	go func() {
		ticker := time.NewTicker(cfg.syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := fetchAndUpdate(cfg.consumerAuthURL, store); err != nil {
					log.Printf("PIP: sync error: %v", err)
				}
			}
		}
	}()

	srv := NewServer(store, cfg.consumerAuthURL)
	addr := ":" + cfg.port
	log.Printf("PIP: listening on %s (sync interval %s)", addr, cfg.syncInterval)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("PIP: %v", err)
	}
}
