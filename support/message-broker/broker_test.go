package broker_test

import (
	"os"
	"testing"
	"time"

	"arrowhead/message-broker"
)

// TestNewBrokerUnreachable verifies that New returns a meaningful error
// when the AMQP server is not reachable. This test never needs RabbitMQ.
func TestNewBrokerUnreachable(t *testing.T) {
	_, err := broker.New(broker.Config{URL: "amqp://guest:guest@127.0.0.1:1/"})
	if err == nil {
		t.Fatal("expected error connecting to closed port")
	}
}

// TestNewBrokerEmptyURL verifies that New returns an error for an empty URL.
func TestNewBrokerEmptyURL(t *testing.T) {
	_, err := broker.New(broker.Config{URL: ""})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

// TestPublishSubscribeRoundTrip publishes a message and verifies it is received.
// Requires a real RabbitMQ instance; skipped when AMQP_URL is not set.
func TestPublishSubscribeRoundTrip(t *testing.T) {
	url := os.Getenv("AMQP_URL")
	if url == "" {
		t.Skip("AMQP_URL not set — skipping RabbitMQ integration test")
	}

	b, err := broker.New(broker.Config{URL: url, Exchange: "test-arrowhead"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	received := make(chan []byte, 1)
	if err := b.Subscribe("test-queue", "test.key", func(payload []byte) {
		received <- payload
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	want := []byte(`{"msg":"hello"}`)
	if err := b.Publish("test.key", want); err != nil {
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
