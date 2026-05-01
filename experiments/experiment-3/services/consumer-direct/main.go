package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	broker "arrowhead/message-broker"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func run(amqpURL, name, queue, bindingKey string) error {
	b, err := broker.New(broker.Config{URL: amqpURL, Exchange: "arrowhead"})
	if err != nil {
		return err
	}
	defer b.Close()

	if err := b.Subscribe(queue, bindingKey, func(payload []byte) {
		log.Printf("[%s] received: %s", name, payload)
	}); err != nil {
		return err
	}

	log.Printf("[%s] subscribed with binding key %q — waiting for messages", name, bindingKey)

	// Block until process exits. Reconnection requires restart.
	select {}
}

func main() {
	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		log.Fatal("AMQP_URL is required")
	}

	name := envOr("CONSUMER_NAME", "consumer-direct")
	bindingKey := envOr("BINDING_KEY", "telemetry.#")
	healthPort := envOr("HEALTH_PORT", "9002")
	queue := name + "-queue"

	// Health server — always returns 200.
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	go func() {
		log.Printf("[%s] health server on :%s", name, healthPort)
		if err := http.ListenAndServe(":"+healthPort, nil); err != nil {
			log.Fatalf("health server: %v", err)
		}
	}()

	// Retry loop with 3s back-off.
	for {
		if err := run(amqpURL, name, queue, bindingKey); err != nil {
			log.Printf("[%s] error: %v — retrying in 3s", name, err)
			time.Sleep(3 * time.Second)
		}
	}
}
