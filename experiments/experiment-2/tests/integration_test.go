// Integration tests for experiment-2.
//
// These tests require a running RabbitMQ broker.  Set AMQP_URL to enable them:
//
//	AMQP_URL=amqp://guest:guest@localhost:5672/ go test ./...
package tests_test

import (
	"os"
	"testing"
	"time"

	broker "arrowhead/message-broker"
)

// TestPublishSubscribeRoundTrip publishes a message and verifies it is received
// by a subscriber on the same exchange.  Requires AMQP_URL.
func TestPublishSubscribeRoundTrip(t *testing.T) {
	url := os.Getenv("AMQP_URL")
	if url == "" {
		t.Skip("AMQP_URL not set — skipping RabbitMQ integration test")
	}

	b, err := broker.New(broker.Config{URL: url, Exchange: "arrowhead-test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	received := make(chan []byte, 1)
	if err := b.Subscribe("exp2-test-queue", "telemetry.robot", func(payload []byte) {
		received <- payload
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	want := []byte(`{"robotId":"robot-1","temperature":22.5,"seq":1}`)
	if err := b.Publish("telemetry.robot", want); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != string(want) {
			t.Errorf("got %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

// TestWildcardSubscription verifies that "telemetry.#" matches both
// "telemetry.robot" and "telemetry.sensor".
func TestWildcardSubscription(t *testing.T) {
	url := os.Getenv("AMQP_URL")
	if url == "" {
		t.Skip("AMQP_URL not set — skipping RabbitMQ integration test")
	}

	b, err := broker.New(broker.Config{URL: url, Exchange: "arrowhead-test-wildcard"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	received := make(chan string, 10)
	if err := b.Subscribe("exp2-wildcard-queue", "telemetry.#", func(payload []byte) {
		received <- string(payload)
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	keys := []string{"telemetry.robot", "telemetry.sensor", "telemetry.camera"}
	for _, k := range keys {
		if err := b.Publish(k, []byte(`{"key":"`+k+`"}`)); err != nil {
			t.Fatalf("Publish %s: %v", k, err)
		}
	}

	timeout := time.After(5 * time.Second)
	count := 0
	for count < len(keys) {
		select {
		case <-received:
			count++
		case <-timeout:
			t.Fatalf("timed out: received %d/%d messages", count, len(keys))
		}
	}
}
