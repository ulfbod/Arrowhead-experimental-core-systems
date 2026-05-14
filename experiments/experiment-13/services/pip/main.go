package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

type config struct {
	port           string
	profileCAGRPC  string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func configFromEnv() config {
	return config{
		port:          envOr("PORT", "9306"),
		profileCAGRPC: envOr("PROFILE_CA_GRPC_ADDR", "profile-ca:8089"),
	}
}

func main() {
	cfg := configFromEnv()
	store := NewSubjectStore()

	// Start the gRPC subscriber goroutine before the HTTP server.
	sub := newSubscriber(store, newGRPCDialFn(cfg.profileCAGRPC), time.Second)
	go sub.run(context.Background())

	srv := NewServer(store)
	addr := ":" + cfg.port
	log.Printf("PIP: listening on %s (profile-ca gRPC: %s)", addr, cfg.profileCAGRPC)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("PIP: %v", err)
	}
}
