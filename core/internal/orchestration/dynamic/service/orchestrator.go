// Package service implements DynamicServiceOrchestration business logic.
//
// AH5 responsibility: find matching service instances by dynamically querying
// the ServiceRegistry and (optionally) checking ConsumerAuthorization.
//
// Strategy: "dynamic" — real-time lookup, no pre-configured rules.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/orchestration/dynamic/client"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

var (
	ErrMissingRequester = errors.New("requesterSystem.systemName is required")
	ErrMissingService   = errors.New("requestedService.serviceDefinition is required")
	// ErrIdentityRequired is returned when ENABLE_IDENTITY_CHECK=true and no token was provided.
	ErrIdentityRequired = errors.New("identity token required: provide Authorization: Bearer <token>")
	// ErrIdentityInvalid is returned when the token is expired, unknown, or the Authentication system is unreachable.
	ErrIdentityInvalid = errors.New("identity token is invalid or expired")
)

// DynamicOrchestrator performs real-time orchestration.
type DynamicOrchestrator struct {
	srClient          client.ServiceRegistryClient
	caClient          client.ConsumerAuthClient
	idClient          client.IdentityClient // nil when checkIdentity=false
	blClient          blclient.BlacklistClient
	qosClient         client.QoSEvaluatorClient     // nil → NopQoSClient (fail-open)
	translationClient client.TranslationClient      // nil → NopTranslationClient (ALLOW_TRANSLATION is no-op)
	lockChecker       LockChecker                   // nil → NopLockChecker (ONLY_EXCLUSIVE is no-op)
	checkAuth         bool
	checkIdentity     bool
	hist              *historyStore
	pushClient        *http.Client // used for push notification delivery; nil → http.DefaultClient
}

// NewDynamicOrchestratorWithClients creates a new orchestrator with injected interface
// implementations. Use in tests and for dependency injection.
// blClient is consulted to exclude blacklisted providers and reject blacklisted requesters.
// Pass blclient.NopClient{} to disable blacklist filtering.
func NewDynamicOrchestratorWithClients(
	srClient client.ServiceRegistryClient,
	caClient client.ConsumerAuthClient,
	idClient client.IdentityClient,
	blClient blclient.BlacklistClient,
	checkAuth, checkIdentity bool,
) *DynamicOrchestrator {
	return &DynamicOrchestrator{
		srClient:      srClient,
		caClient:      caClient,
		idClient:      idClient,
		blClient:      blClient,
		qosClient:     client.NopQoSClient{},
		checkAuth:     checkAuth,
		checkIdentity: checkIdentity,
		hist:          newHistoryStore(),
	}
}

// SetQoSClient configures the QoS evaluator client used for latency-based filtering.
// Pass client.NopQoSClient{} to disable QoS filtering (fail-open).
func (o *DynamicOrchestrator) SetQoSClient(c client.QoSEvaluatorClient) { o.qosClient = c }

// SetTranslationClient configures the Translation Manager client (G36).
// When set and ALLOW_TRANSLATION flag is true, providers that fail interface matching
// are re-evaluated via CanTranslate and included if translation is possible.
func (o *DynamicOrchestrator) SetTranslationClient(c client.TranslationClient) {
	o.translationClient = c
}

// SetPushClient configures the HTTP client used for push notification delivery.
// Pass a client with the desired timeout (e.g. derived from PUSH_DELIVERY_TIMEOUT_SECONDS).
func (o *DynamicOrchestrator) SetPushClient(c *http.Client) { o.pushClient = c }

// SetLockChecker configures the lock checker used for ONLY_EXCLUSIVE flag filtering (G48).
// Pass NopLockChecker{} to disable exclusive-lock filtering (fail-open).
func (o *DynamicOrchestrator) SetLockChecker(lc LockChecker) { o.lockChecker = lc }

// QueryHistory returns orchestration history entries matching the given filter.
// Pass a zero-value HistoryQueryFilter{} to return all entries.
func (o *DynamicOrchestrator) QueryHistory(f HistoryQueryFilter) HistoryQueryResponse {
	return o.hist.query(f)
}

// TriggerPush records a PUSH/PENDING history entry and asynchronously delivers
// the push notification to the subscriber's notifyInterface URL.
// Returns immediately; delivery happens in a goroutine.
func (o *DynamicOrchestrator) TriggerPush(sub Subscription) {
	entryID := o.hist.add(newHistoryEntryTyped(
		sub.OwnerSystemName, sub.TargetSystemName, "PENDING",
		"triggered for subscription "+sub.ID,
		"PUSH",
	))
	go o.deliverPush(sub, entryID)
}

// deliverPush posts the orchestration result to the subscriber's notify URL and
// updates the history entry to DELIVERED or FAILED.
func (o *DynamicOrchestrator) deliverPush(sub Subscription, entryID string) {
	notifyURL := extractNotifyURL(sub.NotifyInterface)
	if notifyURL == "" {
		o.hist.updateStatus(entryID, "FAILED")
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"subscriptionId":  sub.ID,
		"ownerSystemName": sub.OwnerSystemName,
		"targetSystemName": sub.TargetSystemName,
	})
	httpClient := o.pushClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Post(notifyURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		o.hist.updateStatus(entryID, "FAILED")
		return
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		o.hist.updateStatus(entryID, "FAILED")
		return
	}
	o.hist.updateStatus(entryID, "DELIVERED")
}

// interfacesMatch returns true if any of the provider's interfaces appears in the requested list.
func interfacesMatch(providerIfaces, requestedIfaces []string) bool {
	for _, p := range providerIfaces {
		for _, r := range requestedIfaces {
			if p == r {
				return true
			}
		}
	}
	return false
}

// extractNotifyURL returns the delivery URL from a notifyInterface map.
// Tries "notifyUri", then "uri", then assembles from "address"+"port"+"path".
func extractNotifyURL(ni map[string]any) string {
	for _, key := range []string{"notifyUri", "uri"} {
		if v, ok := ni[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	addr, _ := ni["address"].(string)
	path, _ := ni["path"].(string)
	if addr == "" {
		return ""
	}
	port := ""
	switch p := ni["port"].(type) {
	case float64:
		port = strconv.Itoa(int(p))
	case string:
		port = p
	}
	if port == "" {
		return "http://" + addr + path
	}
	return "http://" + addr + ":" + port + path
}

// Orchestrate performs the pull operation: optionally verify identity, query SR,
// optionally check ConsumerAuthorization, and return results.
//
// token is the Bearer token from the Authorization header (empty string if absent).
// When checkIdentity=true, an empty token returns ErrIdentityRequired; an invalid
// or expired token returns ErrIdentityInvalid. On success, the verified systemName
// from the token replaces req.RequesterSystem.SystemName for all downstream checks.
func (o *DynamicOrchestrator) Orchestrate(req orchmodel.OrchestrationRequest, token string) (orchmodel.OrchestrationResponse, error) {
	ctx := context.Background()

	// Step 1: Identity verification (beyond AH5 spec — see GAP_ANALYSIS.md D8).
	if o.checkIdentity {
		if token == "" {
			return orchmodel.OrchestrationResponse{}, ErrIdentityRequired
		}
		verifiedName, err := o.idClient.VerifyToken(ctx, token)
		if err != nil {
			return orchmodel.OrchestrationResponse{}, ErrIdentityInvalid
		}
		// Override self-reported name with the cryptographically verified identity.
		req.RequesterSystem.SystemName = verifiedName
	}

	// Step 2: Validate request fields.
	if req.OrchestrationFlags.AllowIntercloud || req.OrchestrationFlags.OnlyIntercloud {
		return orchmodel.OrchestrationResponse{}, orchmodel.ErrInterclouNotSupported
	}
	if req.RequesterSystem.SystemName == "" {
		return orchmodel.OrchestrationResponse{}, ErrMissingRequester
	}
	if req.RequestedService.ServiceDefinition == "" {
		return orchmodel.OrchestrationResponse{}, ErrMissingService
	}

	// Step 2.5: Reject blacklisted requester (fail-closed).
	if blacklisted, _ := o.blClient.IsBlacklisted(ctx, req.RequesterSystem.SystemName); blacklisted {
		return orchmodel.OrchestrationResponse{}, fmt.Errorf("requester system is blacklisted")
	}

	// Step 3: Query Service Registry via SR client interface.
	srResults, err := o.srClient.LookupServices(ctx, req)
	if err != nil {
		return orchmodel.OrchestrationResponse{}, fmt.Errorf("service registry unreachable: %w", err)
	}

	// Step 4: Filter by ConsumerAuthorization (optional) and blacklist.
	var results []orchmodel.OrchestrationResult
	for _, r := range srResults {
		if o.checkAuth {
			ok, err := o.caClient.IsAuthorized(ctx, req.RequesterSystem.SystemName, r.ProviderName, r.ServiceDefinition)
			if err != nil || !ok {
				continue
			}
		}
		// Exclude blacklisted providers (fail-closed).
		if blacklisted, _ := o.blClient.IsBlacklisted(ctx, r.ProviderName); blacklisted {
			continue
		}
		results = append(results, r)
	}
	if results == nil {
		results = []orchmodel.OrchestrationResult{}
	}

	// Step 4.3: Interface filtering with ALLOW_TRANSLATION support (G36).
	// When the request specifies interfaces and ALLOW_TRANSLATION is true and a
	// TranslationClient is configured, providers that fail interface matching are
	// re-evaluated via CanTranslate. Fail-open: translation check error → include.
	if len(req.RequestedService.Interfaces) > 0 && req.OrchestrationFlags.AllowTranslation && o.translationClient != nil {
		tc := o.translationClient
		filtered := make([]orchmodel.OrchestrationResult, 0, len(results))
		for _, r := range results {
			if interfacesMatch(r.Interfaces, req.RequestedService.Interfaces) {
				filtered = append(filtered, r)
				continue
			}
			// Interface mismatch — check if translation is possible.
			canTranslate := false
			for _, provIface := range r.Interfaces {
				for _, reqIface := range req.RequestedService.Interfaces {
					ok, err := tc.CanTranslate(ctx, provIface, reqIface)
					if err == nil && ok {
						canTranslate = true
					}
				}
			}
			if canTranslate {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Step 4.5: QoS filtering (G40 + G53). Applied only when qualityRequirements are specified.
	if len(req.QualityRequirements) > 0 {
		qosClient := o.qosClient
		if qosClient == nil {
			qosClient = client.NopQoSClient{}
		}
		// Collect the strictest thresholds across all requirements.
		var maxLatencyMs int64 = -1
		var maxBandwidthBps int64 // minimum bandwidth threshold (counter-intuitive spec naming)
		var maxJitterMs int64 = -1
		var maxPacketLoss float64 = -1
		for _, qr := range req.QualityRequirements {
			if qr.MaxLatencyMs > 0 && (maxLatencyMs < 0 || qr.MaxLatencyMs < maxLatencyMs) {
				maxLatencyMs = qr.MaxLatencyMs
			}
			if qr.MaxBandwidthBps > maxBandwidthBps {
				maxBandwidthBps = qr.MaxBandwidthBps
			}
			if qr.MaxJitterMs > 0 && (maxJitterMs < 0 || qr.MaxJitterMs < maxJitterMs) {
				maxJitterMs = qr.MaxJitterMs
			}
			if qr.MaxPacketLoss >= 0 && (maxPacketLoss < 0 || qr.MaxPacketLoss < maxPacketLoss) {
				maxPacketLoss = qr.MaxPacketLoss
			}
		}

		needFullMeasure := maxBandwidthBps > 0 || maxJitterMs >= 0 || maxPacketLoss >= 0

		filtered := results[:0]
		for _, r := range results {
			if needFullMeasure {
				// Use FullMeasure to get all QoS dimensions.
				rec, err := qosClient.FullMeasure(ctx, r.ProviderAddress, strconv.Itoa(r.ProviderPort))
				if err != nil {
					// QoS evaluator unreachable → fail-open.
					filtered = append(filtered, r)
					continue
				}
				if !rec.Reachable {
					continue
				}
				if maxLatencyMs >= 0 && rec.LatencyMs > maxLatencyMs {
					continue
				}
				// MaxBandwidthBps is a *minimum* bandwidth requirement (spec naming quirk).
				if maxBandwidthBps > 0 && rec.BandwidthBps < maxBandwidthBps {
					continue
				}
				if maxJitterMs >= 0 && rec.JitterMs > maxJitterMs {
					continue
				}
				if maxPacketLoss >= 0 && rec.PacketLoss > maxPacketLoss {
					continue
				}
				filtered = append(filtered, r)
			} else {
				// Latency-only path (backward-compatible with G40).
				latency, reachable, err := qosClient.Measure(ctx, r.ProviderAddress, strconv.Itoa(r.ProviderPort))
				if err != nil {
					filtered = append(filtered, r)
					continue
				}
				if !reachable {
					continue
				}
				if maxLatencyMs >= 0 && latency > maxLatencyMs {
					continue
				}
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Step 5: Apply orchestration flags.
	flags := req.OrchestrationFlags
	if flags.OnlyPreferred && len(req.PreferredProviders) > 0 {
		preferred := make(map[string]bool, len(req.PreferredProviders))
		for _, p := range req.PreferredProviders {
			preferred[p.SystemName] = true
		}
		filtered := results[:0]
		for _, r := range results {
			if preferred[r.ProviderName] {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Step 5.1: ONLY_EXCLUSIVE filter (G48). When true, exclude providers that are
	// currently under an active exclusive lock according to the LockChecker.
	if flags.OnlyExclusive {
		lc := o.lockChecker
		if lc == nil {
			lc = NopLockChecker{}
		}
		filtered := results[:0]
		for _, r := range results {
			if !lc.IsLocked(r.ProviderName) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if flags.Matchmaking && len(results) > 1 {
		results = results[:1]
	}

	resp := orchmodel.OrchestrationResponse{Results: results}
	o.hist.add(newHistoryEntry(
		req.RequesterSystem.SystemName,
		req.RequestedService.ServiceDefinition,
		"DONE", "",
	))
	return resp, nil
}
