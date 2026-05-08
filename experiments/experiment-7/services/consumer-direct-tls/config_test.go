package main

import (
	"strings"
	"testing"
)

// TestRequireHTTPS verifies that requireHTTPS rejects plain http:// URLs and
// accepts https:// or empty (optional) URLs.
// These tests enforce experiment-7's constraint: all inter-system calls to core
// services must use mTLS (https://) — plain HTTP is not permitted.
func TestRequireHTTPS(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		url     string
		wantErr bool
	}{
		{"https URL accepted", "AUTH_URL", "https://authentication:8481", false},
		{"https URL with path accepted", "ORCHESTRATION_URL", "https://dynamicorch:8483/orchestration/dynamic", false},
		{"empty URL accepted (optional field)", "SR_URL", "", false},
		{"http URL rejected", "AUTH_URL", "http://authentication:8081", true},
		{"http URL rejected for orchestration", "ORCHESTRATION_URL", "http://dynamicorch:8083", true},
		{"http localhost rejected", "AUTH_URL", "http://localhost:8081", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireHTTPS(tt.envName, tt.url)
			if tt.wantErr && err == nil {
				t.Errorf("requireHTTPS(%q, %q): expected error, got nil", tt.envName, tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("requireHTTPS(%q, %q): unexpected error: %v", tt.envName, tt.url, err)
			}
			// Error message should mention the env var name for useful diagnostics.
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.envName) {
				t.Errorf("error should mention env var %q, got: %v", tt.envName, err)
			}
		})
	}
}
