// client.go — polling REST client for service-partner.
//
// PollClient polls the pki-rest-authz mTLS proxy for telemetry/latest
// and tracks message statistics.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks polling metrics.
type Stats struct {
	mu           sync.Mutex
	msgCount     atomic.Int64
	deniedCount  atomic.Int64
	lastReceived string
	lastDeniedAt string
	transport    string
}

// NewStats creates a Stats with the given transport label.
func NewStats(transport string) *Stats {
	return &Stats{transport: transport}
}

func (s *Stats) recordSuccess() {
	s.msgCount.Add(1)
	s.mu.Lock()
	s.lastReceived = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
}

func (s *Stats) recordDenied() {
	s.deniedCount.Add(1)
	s.mu.Lock()
	s.lastDeniedAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
}

// Snapshot returns a JSON-serialisable snapshot.
func (s *Stats) Snapshot() map[string]interface{} {
	s.mu.Lock()
	recv := s.lastReceived
	denied := s.lastDeniedAt
	s.mu.Unlock()
	return map[string]interface{}{
		"msgCount":     s.msgCount.Load(),
		"deniedCount":  s.deniedCount.Load(),
		"lastReceived": recv,
		"lastDeniedAt": denied,
		"transport":    s.transport,
	}
}

// PollClient polls the upstream REST endpoint via mTLS.
type PollClient struct {
	client      *http.Client
	targetURL   string
	serviceName string
	stats       *Stats
}

// NewPollClient creates a PollClient using the given TLS config.
func NewPollClient(cert tls.Certificate, caPool *x509.CertPool, targetURL, serviceName string) *PollClient {
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
	return &PollClient{
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   10 * time.Second,
		},
		targetURL:   targetURL,
		serviceName: serviceName,
		stats:       NewStats("rest-mtls-pki"),
	}
}

// Stats returns the polling statistics.
func (c *PollClient) Stats() *Stats { return c.stats }

// PollOnce sends one GET /telemetry/latest request.
// Returns (body, nil) on success, ("", err) on failure.
func (c *PollClient) PollOnce() ([]byte, error) {
	url := c.targetURL + "/telemetry/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.serviceName != "" {
		req.Header.Set("X-Service-Name", c.serviceName)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		c.stats.recordSuccess()
		return body, nil
	case http.StatusForbidden:
		c.stats.recordDenied()
		return nil, fmt.Errorf("403 Forbidden — check ConsumerAuth grants for this service partner")
	default:
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
}

// RunPoll runs a poll loop until the done channel is closed.
func (c *PollClient) RunPoll(interval time.Duration, done <-chan struct{}) {
	log.Printf("[service-partner] polling %s every %s", c.targetURL, interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			body, err := c.PollOnce()
			if err != nil {
				log.Printf("[service-partner] poll error: %v", err)
			} else {
				log.Printf("[service-partner] received %d bytes", len(body))
			}
		}
	}
}

// HandleHealth writes a JSON health response.
func HandleHealth(partnerName string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","partner":%q}`, partnerName)
	}
}

// HandleStats writes polling stats as JSON.
func HandleStats(stats *Stats) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats.Snapshot())
	}
}
