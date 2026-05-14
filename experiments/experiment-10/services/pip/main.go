package main

import (
	"log"
	"net/http"
	"os"
)

type config struct {
	port string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func configFromEnv() config {
	return config{
		port: envOr("PORT", "9306"),
	}
}

func main() {
	cfg := configFromEnv()
	store := NewSubjectStore()
	srv := NewServer(store)

	addr := ":" + cfg.port
	log.Printf("PIP: listening on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("PIP: %v", err)
	}
}
