package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// ArrowheadClient handles communication with the Arrowhead core
type ArrowheadClient struct {
	BaseURL    string
	ConsumerID string
}

func NewArrowheadClient(baseURL, consumerID string) *ArrowheadClient {
	return &ArrowheadClient{BaseURL: baseURL, ConsumerID: consumerID}
}

// Register registers this service with Arrowhead
func (c *ArrowheadClient) Register(req RegisterRequest) error {
	data, _ := json.Marshal(req)
	resp, err := httpClient.Post(c.BaseURL+"/registry/register", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register failed: %s", string(body))
	}
	return nil
}

// Discover finds a provider for the given service name
func (c *ArrowheadClient) Discover(serviceName string) (*OrchestrationResponse, error) {
	req := OrchestrationRequest{ConsumerID: c.ConsumerID, ServiceName: serviceName}
	data, _ := json.Marshal(req)
	resp, err := httpClient.Post(c.BaseURL+"/orchestration", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	defer resp.Body.Close()
	var orch OrchestrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&orch); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if !orch.Allowed {
		return nil, fmt.Errorf("not authorized: %s", orch.Reason)
	}
	return &orch, nil
}

// CallService makes an authenticated HTTP call via Arrowhead orchestration
func (c *ArrowheadClient) CallService(serviceName, method, path string, body interface{}, result interface{}) error {
	orch, err := c.Discover(serviceName)
	if err != nil {
		return err
	}
	url := orch.Endpoint + path
	return DoRequest(method, url, orch.AuthToken, c.ConsumerID, body, result)
}

// DoRequest makes a raw HTTP request with auth headers
func DoRequest(method, url, token, consumerID string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Auth-Token", token)
	}
	if consumerID != "" {
		req.Header.Set("X-Consumer-ID", consumerID)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// RegisterWithRetry registers with arrowhead, retrying until success
func RegisterWithRetry(client *ArrowheadClient, req RegisterRequest, maxRetries int) {
	for i := 0; i < maxRetries; i++ {
		if err := client.Register(req); err != nil {
			log.Printf("Registration attempt %d failed: %v (retrying in 2s)", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("Registered %s with Arrowhead", req.ID)
		return
	}
	log.Printf("WARNING: Could not register %s with Arrowhead after %d attempts", req.ID, maxRetries)
}
