package service

import (
	"net/http"
	"time"

	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/orchestration/dynamic/client"
)

// NewDynamicOrchestrator creates a new orchestrator with a default http.Client.
//
//   - srURL:         ServiceRegistry base URL
//   - caURL:         ConsumerAuthorization base URL (used when checkAuth=true)
//   - authSysURL:    Authentication system base URL (used when checkIdentity=true)
//   - checkAuth:     when true, filters providers through ConsumerAuthorization
//   - checkIdentity: when true, requires a valid Bearer token and uses the verified
//     systemName from the token instead of the self-reported requesterSystem.systemName
func NewDynamicOrchestrator(srURL, caURL, authSysURL string, checkAuth, checkIdentity bool) *DynamicOrchestrator {
	return NewDynamicOrchestratorWithClient(srURL, caURL, authSysURL, checkAuth, checkIdentity,
		&http.Client{Timeout: 5 * time.Second})
}

// NewDynamicOrchestratorWithClient creates a new orchestrator with a custom
// http.Client.  Use this when the upstream core services are behind TLS and
// the caller must present a client certificate (mutual TLS).
// Blacklist filtering is disabled (NopClient); use NewDynamicOrchestratorWithClients
// directly to wire up a BlacklistClient.
func NewDynamicOrchestratorWithClient(srURL, caURL, authSysURL string, checkAuth, checkIdentity bool, httpCl *http.Client) *DynamicOrchestrator {
	var idClient client.IdentityClient
	if checkIdentity {
		idClient = client.NewIdentityHTTPClient(authSysURL, httpCl)
	}
	return NewDynamicOrchestratorWithClients(
		client.NewSRHTTPClient(srURL, httpCl),
		client.NewCAHTTPClient(caURL, httpCl),
		idClient,
		blclient.NopClient{},
		checkAuth, checkIdentity,
	)
}
