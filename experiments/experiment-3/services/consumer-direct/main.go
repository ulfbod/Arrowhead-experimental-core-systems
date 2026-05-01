package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	gosync "sync"
	"sync/atomic"
	"time"

	broker "arrowhead/message-broker"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// statsTracker counts received messages and records the last receipt time.
type statsTracker struct {
	msgCount       atomic.Int64
	mu             gosync.Mutex
	lastReceivedAt string
}

func (st *statsTracker) record() {
	st.msgCount.Add(1)
	st.mu.Lock()
	st.lastReceivedAt = time.Now().UTC().Format(time.RFC3339)
	st.mu.Unlock()
}

func (st *statsTracker) snapshot(name string) map[string]interface{} {
	st.mu.Lock()
	last := st.lastReceivedAt
	st.mu.Unlock()
	return map[string]interface{}{
		"name":           name,
		"msgCount":       st.msgCount.Load(),
		"lastReceivedAt": last,
	}
}

func run(amqpURL, name, queue, bindingKey string, st *statsTracker) error {
	b, err := broker.New(broker.Config{URL: amqpURL, Exchange: "arrowhead"})
	if err != nil {
		return err
	}
	defer b.Close()

	if err := b.Subscribe(queue, bindingKey, func(payload []byte) {
		st.record()
		log.Printf("[%s] received: %s", name, payload)
	}); err != nil {
		return err
	}

	log.Printf("[%s] subscribed with binding key %q — waiting for messages", name, bindingKey)

	// Block until the connection drops, then return so the retry loop reconnects.
	<-b.Done()
	return fmt.Errorf("connection closed")
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

	st := &statsTracker{}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st.snapshot(name))
	})

	go func() {
		log.Printf("[%s] health server on :%s", name, healthPort)
		if err := http.ListenAndServe(":"+healthPort, nil); err != nil {
			log.Fatalf("health server: %v", err)
		}
	}()

	// Retry loop with 3s back-off.
	for {
		if err := run(amqpURL, name, queue, bindingKey, st); err != nil {
			log.Printf("[%s] error: %v — retrying in 3s", name, err)
			time.Sleep(3 * time.Second)
		}
	}
}
