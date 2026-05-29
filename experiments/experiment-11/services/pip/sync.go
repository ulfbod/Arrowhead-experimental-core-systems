package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// fetchAndUpdate fetches grants from ConsumerAuth at caURL and updates the store.
// Returns an error if the HTTP call fails or the response is not 200.
func fetchAndUpdate(caURL string, store *GrantStore) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(caURL + "/consumerauthorization/authorization/lookup")
	if err != nil {
		return fmt.Errorf("GET ConsumerAuth: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ConsumerAuth returned %d", resp.StatusCode)
	}
	var lr LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return fmt.Errorf("decode LookupResponse: %w", err)
	}
	store.Update(lr.Rules)
	return nil
}
