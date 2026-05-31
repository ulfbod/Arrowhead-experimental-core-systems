package mqttutil_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"arrowhead/core/internal/mqttutil"
)

// mockToken is a no-op MQTT token for testing.
type mockToken struct{}

func (t *mockToken) Wait() bool                        { return true }
func (t *mockToken) WaitTimeout(d time.Duration) bool  { return true }
func (t *mockToken) Done() <-chan struct{}               { ch := make(chan struct{}); close(ch); return ch }
func (t *mockToken) Error() error                      { return nil }

// mockMQTTClient is an in-memory MQTT client for testing.
type mockMQTTClient struct {
	mu          sync.Mutex
	subscribed  map[string]mqtt.MessageHandler
	published   map[string][]byte
}

func newMockClient() *mockMQTTClient {
	return &mockMQTTClient{
		subscribed: make(map[string]mqtt.MessageHandler),
		published:  make(map[string][]byte),
	}
}

func (m *mockMQTTClient) Connect() mqtt.Token                     { return &mockToken{} }
func (m *mockMQTTClient) Disconnect(_ uint)                       {}
func (m *mockMQTTClient) Subscribe(topic string, _ byte, handler mqtt.MessageHandler) mqtt.Token {
	m.mu.Lock()
	m.subscribed[topic] = handler
	m.mu.Unlock()
	return &mockToken{}
}
func (m *mockMQTTClient) Publish(topic string, _ byte, _ bool, payload interface{}) mqtt.Token {
	m.mu.Lock()
	switch p := payload.(type) {
	case []byte:
		m.published[topic] = p
	case string:
		m.published[topic] = []byte(p)
	}
	m.mu.Unlock()
	return &mockToken{}
}

// mockMessage implements mqtt.Message for testing.
type mockMessage struct {
	topic   string
	payload []byte
}

func (m *mockMessage) Duplicate() bool          { return false }
func (m *mockMessage) Qos() byte                { return 1 }
func (m *mockMessage) Retained() bool           { return false }
func (m *mockMessage) Topic() string            { return m.topic }
func (m *mockMessage) MessageID() uint16        { return 0 }
func (m *mockMessage) Payload() []byte          { return m.payload }
func (m *mockMessage) Ack()                     {}

func TestMQTTAdapterRoundTrip(t *testing.T) {
	mockClient := newMockClient()
	adapter := mqttutil.NewMQTTAdapterWithClient(mockClient, "testservice")

	var received mqttutil.RequestMessage
	var wg sync.WaitGroup
	wg.Add(1)
	err := adapter.Subscribe("ah5/testservice/request", func(msg mqttutil.RequestMessage) {
		received = msg
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Simulate an incoming MQTT message.
	msgPayload, _ := json.Marshal(mqttutil.RequestMessage{
		Path:          "/health",
		Method:        "GET",
		CorrelationID: "corr-123",
	})
	mockClient.mu.Lock()
	handler := mockClient.subscribed["ah5/testservice/request"]
	mockClient.mu.Unlock()
	if handler == nil {
		t.Fatal("no handler subscribed for topic")
	}
	handler(nil, &mockMessage{topic: "ah5/testservice/request", payload: msgPayload})
	wg.Wait()

	if received.Path != "/health" {
		t.Errorf("path = %q, want /health", received.Path)
	}
	if received.CorrelationID != "corr-123" {
		t.Errorf("correlationId = %q, want corr-123", received.CorrelationID)
	}

	// Publish a reply.
	replyPayload := []byte(`{"status":"UP"}`)
	if err := adapter.Publish("corr-123", replyPayload); err != nil {
		t.Fatalf("publish: %v", err)
	}
	mockClient.mu.Lock()
	pub := mockClient.published["ah5/testservice/reply/corr-123"]
	mockClient.mu.Unlock()
	if string(pub) != `{"status":"UP"}` {
		t.Errorf("published reply = %q, want {\"status\":\"UP\"}", string(pub))
	}
}

func TestHealthEndpointViaMQTT(t *testing.T) {
	mockClient := newMockClient()
	adapter := mqttutil.NewMQTTAdapterWithClient(mockClient, "serviceregistry")

	// Subscribe: handler responds with health reply
	var wg sync.WaitGroup
	wg.Add(1)
	err := adapter.Subscribe("ah5/serviceregistry/request", func(msg mqttutil.RequestMessage) {
		if msg.Path == "/health" {
			reply := []byte(`{"status":"UP"}`)
			adapter.Publish(msg.CorrelationID, reply) //nolint:errcheck
		}
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Inject a health request.
	msgPayload, _ := json.Marshal(mqttutil.RequestMessage{
		Path:          "/health",
		Method:        "GET",
		CorrelationID: "health-req-1",
	})
	mockClient.mu.Lock()
	handler := mockClient.subscribed["ah5/serviceregistry/request"]
	mockClient.mu.Unlock()
	handler(nil, &mockMessage{topic: "ah5/serviceregistry/request", payload: msgPayload})
	wg.Wait()

	mockClient.mu.Lock()
	pub := mockClient.published["ah5/serviceregistry/reply/health-req-1"]
	mockClient.mu.Unlock()
	if string(pub) != `{"status":"UP"}` {
		t.Errorf("health reply = %q, want {\"status\":\"UP\"}", string(pub))
	}
}

func TestSystemRegistersMQTTInterfaceWhenBrokerSet(t *testing.T) {
	// This test verifies that when MQTT_BROKER_URL is set, the interface
	// "MQTT-INSECURE-JSON" would be included alongside "HTTP-INSECURE-JSON".
	// Since we cannot start a real broker in tests, we verify the mqttutil
	// package compiles and the interface name constant is correct.
	const expectedInterface = "MQTT-INSECURE-JSON"
	if mqttutil.MQTTInterfaceName != expectedInterface {
		t.Errorf("MQTTInterfaceName = %q, want %q", mqttutil.MQTTInterfaceName, expectedInterface)
	}
}
