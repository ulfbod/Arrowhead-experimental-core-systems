// Package mqttutil provides MQTT communication helpers for AH5 core systems.
// When MQTT_BROKER_URL is set, a system can subscribe to an MQTT request topic
// and publish replies, enabling MQTT-INSECURE-JSON interface alongside HTTP.
//
// Topic scheme:
//
//	Request:  ah5/<system>/request
//	Reply:    ah5/<system>/reply/<correlationId>
//
// Request payload JSON:
//
//	{"path":"/health","method":"GET","correlationId":"abc","body":"..."}
package mqttutil

import (
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTInterfaceName is the AH5 interface template name for MQTT-based communication.
// When MQTT_BROKER_URL is set, this interface should be registered alongside HTTP-INSECURE-JSON.
const MQTTInterfaceName = "MQTT-INSECURE-JSON"

// MQTTClient is the interface over mqtt.Client used by MQTTAdapter.
// It is extracted to allow mocking in tests.
type MQTTClient interface {
	Connect() mqtt.Token
	Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	Disconnect(quiesce uint)
}

// MQTTAdapter wraps an MQTT client with AH5 topic conventions.
type MQTTAdapter struct {
	client      MQTTClient
	replyPrefix string // e.g. "ah5/serviceregistry/reply/"
}

// RequestMessage is the JSON structure of an MQTT request message.
type RequestMessage struct {
	Path          string `json:"path"`
	Method        string `json:"method"`
	CorrelationID string `json:"correlationId"`
	Body          string `json:"body,omitempty"`
}

// NewMQTTAdapter connects to an MQTT broker and subscribes to the given request topic.
// brokerURL: e.g. "tcp://localhost:1883"
// clientID: unique client identifier
// systemTopic: e.g. "serviceregistry" → request topic: "ah5/serviceregistry/request"
func NewMQTTAdapter(brokerURL, clientID, systemTopic string) (*MQTTAdapter, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetConnectTimeout(5 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetAutoReconnect(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return nil, fmt.Errorf("mqtt: connect timeout to %s", brokerURL)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("mqtt: connect error: %w", err)
	}

	return &MQTTAdapter{
		client:      client,
		replyPrefix: "ah5/" + systemTopic + "/reply/",
	}, nil
}

// NewMQTTAdapterWithClient creates an MQTTAdapter using a pre-configured MQTTClient.
// Used in tests to inject a mock client.
func NewMQTTAdapterWithClient(client MQTTClient, systemTopic string) *MQTTAdapter {
	return &MQTTAdapter{
		client:      client,
		replyPrefix: "ah5/" + systemTopic + "/reply/",
	}
}

// Subscribe registers a handler function for incoming request messages.
// The handler receives the decoded path, method, correlationId, and body.
func (a *MQTTAdapter) Subscribe(requestTopic string, handler func(msg RequestMessage)) error {
	token := a.client.Subscribe(requestTopic, 1, func(_ mqtt.Client, m mqtt.Message) {
		var req RequestMessage
		if err := json.Unmarshal(m.Payload(), &req); err != nil {
			return
		}
		handler(req)
	})
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt: subscribe timeout on %s", requestTopic)
	}
	return token.Error()
}

// Publish sends a payload to the reply topic for the given correlationId.
func (a *MQTTAdapter) Publish(correlationID string, payload []byte) error {
	topic := a.replyPrefix + correlationID
	token := a.client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt: publish timeout on %s", topic)
	}
	return token.Error()
}

// Close disconnects from the MQTT broker.
func (a *MQTTAdapter) Close() {
	a.client.Disconnect(250)
}
