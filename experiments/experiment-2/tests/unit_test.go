// Package tests contains unit tests for experiment-2 components.
// These tests do not require any external services.
package tests_test

import (
	"encoding/json"
	"testing"

	broker "arrowhead/message-broker"
)

// TestBrokerRejectsEmptyURL verifies the broker constructor fails for an empty URL.
func TestBrokerRejectsEmptyURL(t *testing.T) {
	_, err := broker.New(broker.Config{URL: ""})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

// TestBrokerRejectsUnreachableURL verifies the broker constructor returns an error
// when the AMQP server is not running on the given address.
func TestBrokerRejectsUnreachableURL(t *testing.T) {
	_, err := broker.New(broker.Config{URL: "amqp://guest:guest@127.0.0.1:1/"})
	if err == nil {
		t.Fatal("expected error for unreachable address")
	}
}

// TestTelemetryPayloadRoundTrip verifies the JSON shape used by robot-simulator
// can be marshaled and unmarshaled without loss.
func TestTelemetryPayloadRoundTrip(t *testing.T) {
	type telemetryMsg struct {
		RobotID     string  `json:"robotId"`
		Temperature float64 `json:"temperature"`
		Humidity    float64 `json:"humidity"`
		Timestamp   string  `json:"timestamp"`
		Seq         int64   `json:"seq"`
	}

	original := telemetryMsg{
		RobotID:     "robot-1",
		Temperature: 22.5,
		Humidity:    55.1,
		Timestamp:   "2024-01-01T00:00:00Z",
		Seq:         42,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded telemetryMsg
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.RobotID != original.RobotID {
		t.Errorf("RobotID: got %q, want %q", decoded.RobotID, original.RobotID)
	}
	if decoded.Temperature != original.Temperature {
		t.Errorf("Temperature: got %v, want %v", decoded.Temperature, original.Temperature)
	}
	if decoded.Seq != original.Seq {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, original.Seq)
	}
}

// TestOrchestrationRequestShape verifies the JSON request body sent by the
// consumer service can be parsed as expected.
func TestOrchestrationRequestShape(t *testing.T) {
	body := map[string]any{
		"requesterSystem": map[string]any{
			"systemName": "demo-consumer",
			"address":    "localhost",
			"port":       9002,
		},
		"requestedService": map[string]any{
			"serviceDefinition": "telemetry",
			"interfaces":        []string{"HTTP-INSECURE-JSON"},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	rs, ok := decoded["requesterSystem"].(map[string]any)
	if !ok {
		t.Fatal("requesterSystem not a map")
	}
	if rs["systemName"] != "demo-consumer" {
		t.Errorf("systemName: got %v", rs["systemName"])
	}

	svc, ok := decoded["requestedService"].(map[string]any)
	if !ok {
		t.Fatal("requestedService not a map")
	}
	if svc["serviceDefinition"] != "telemetry" {
		t.Errorf("serviceDefinition: got %v", svc["serviceDefinition"])
	}
}
