// Package client — QoS evaluator client interface and implementations.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// QoSEvaluatorClient describes what DynamicOrchestration needs from the Device QoS Evaluator.
type QoSEvaluatorClient interface {
	Measure(ctx context.Context, host, port string) (latencyMs int64, reachable bool, err error)
}

// NopQoSClient always returns reachable=true (fail-open). Used when no QoS evaluator is configured.
type NopQoSClient struct{}

// Measure implements QoSEvaluatorClient. Always succeeds (fail-open).
func (NopQoSClient) Measure(_ context.Context, _, _ string) (int64, bool, error) {
	return 0, true, nil
}

// HTTPQoSEvaluatorClient calls a real Device QoS Evaluator over HTTP.
type HTTPQoSEvaluatorClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPQoSEvaluatorClient creates an HTTP-backed QoS evaluator client.
func NewHTTPQoSEvaluatorClient(baseURL string, c *http.Client) *HTTPQoSEvaluatorClient {
	if c == nil {
		c = http.DefaultClient
	}
	return &HTTPQoSEvaluatorClient{baseURL: baseURL, httpClient: c}
}

// Measure calls POST <baseURL>/deviceqosevaluator/quality-evaluation/measure and returns latency+reachability.
func (c *HTTPQoSEvaluatorClient) Measure(ctx context.Context, host, port string) (int64, bool, error) {
	body, _ := json.Marshal(map[string]string{"host": host, "port": port})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/deviceqosevaluator/quality-evaluation/measure",
		bytes.NewReader(body))
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("qos evaluator returned %d", resp.StatusCode)
	}
	var rec struct {
		LatencyMs int64 `json:"latencyMs"`
		Reachable bool  `json:"reachable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		return 0, false, err
	}
	return rec.LatencyMs, rec.Reachable, nil
}
