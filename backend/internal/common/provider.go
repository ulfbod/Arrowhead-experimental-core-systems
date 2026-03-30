package common

import (
	"fmt"
	"log"
	"sync"
	"time"
)

const qosWindowSize = 10

// ProviderConfig defines a candidate provider with its nominal QoS characteristics.
type ProviderConfig struct {
	ID                 string
	URL                string
	Primary            bool
	NominalAccuracy    float64 // 0–1
	NominalLatencyMs   float64 // expected round-trip, ms
	NominalReliability float64 // 0–1
}

type providerHealth struct {
	online         bool
	degraded       bool
	degradeFactor  float64 // accuracy multiplier (0 < x ≤ 1)
	degradeExtraMs float64 // extra latency added when degraded
	consecFails    int
	window         [qosWindowSize]bool
	windowIdx      int
	windowFill     int
	lastRespMs     float64
	lastGoodAt     time.Time
	failedAt       *time.Time // first failure time in current run
	detectedAt     *time.Time // when threshold was hit
}

func (h *providerHealth) record(ok bool, respMs float64) {
	h.lastRespMs = respMs
	h.window[h.windowIdx%qosWindowSize] = ok
	h.windowIdx++
	if h.windowFill < qosWindowSize {
		h.windowFill++
	}
	if ok {
		h.consecFails = 0
		h.online = true
		h.lastGoodAt = time.Now()
		h.failedAt = nil
		h.detectedAt = nil
	} else {
		h.consecFails++
		if h.failedAt == nil {
			t := time.Now()
			h.failedAt = &t
		}
	}
}

func (h *providerHealth) reliability() float64 {
	if h.windowFill == 0 {
		return 1.0
	}
	n := 0
	for i := 0; i < h.windowFill; i++ {
		if h.window[i] {
			n++
		}
	}
	return float64(n) / float64(h.windowFill)
}

// ProviderSelector manages an ordered set of providers for one capability,
// detecting failures automatically and switching to fallbacks.
// It supports two failover modes controlled by common.GetOrchestrationMode():
//   - "local":   immediately switches to the pre-configured fallback (fast)
//   - "central": queries Arrowhead orchestration to obtain the new provider (adds network round-trip)
type ProviderSelector struct {
	mu            sync.Mutex
	capability    string
	cdtID         string
	configs       []ProviderConfig
	health        []*providerHealth
	activeIdx     int
	failThreshold int // consecutive failures before switching provider
	events        []FailoverEvent
	eventLog      *FailoverLogger
	ah            *ArrowheadClient // used in "central" orchestration mode
}

// NewProviderSelector creates a selector. configs[0] is the primary provider.
// ah may be nil; it is only used when orchestration mode is "central".
func NewProviderSelector(cdtID, capability string, configs []ProviderConfig, failThreshold int, evLog *FailoverLogger, ah *ArrowheadClient) *ProviderSelector {
	if failThreshold <= 0 {
		failThreshold = 3
	}
	hs := make([]*providerHealth, len(configs))
	for i := range hs {
		hs[i] = &providerHealth{
			online:        true,
			degradeFactor: 1.0,
			lastGoodAt:    time.Now(),
		}
	}
	return &ProviderSelector{
		capability:    capability,
		cdtID:         cdtID,
		configs:       configs,
		health:        hs,
		activeIdx:     0,
		failThreshold: failThreshold,
		events:        []FailoverEvent{},
		eventLog:      evLog,
		ah:            ah,
	}
}

// Do calls the active provider at method+path, measures latency, updates health,
// and triggers failover + immediate retry when the failure threshold is exceeded.
//
// Failover mode is determined by common.GetOrchestrationMode():
//   - "local":   next provider is chosen from the pre-configured list (zero extra calls)
//   - "central": Arrowhead is queried for the new provider (adds one orchestration round-trip,
//     subject to the global network delay)
//
// Returns (QoSMetrics for this call, error).
func (ps *ProviderSelector) Do(method, path string, body, result interface{}) (QoSMetrics, error) {
	// Phase 1: read active provider without holding lock during HTTP call.
	ps.mu.Lock()
	idx := ps.activeIdx
	cfg := ps.configs[idx]
	ps.mu.Unlock()

	// Phase 2: call active provider.
	start := time.Now()
	err := DoRequest(method, cfg.URL+path, "", ps.cdtID, body, result)
	elapsed := float64(time.Since(start).Milliseconds())

	// Phase 3: update health; decide whether to fail over.
	ps.mu.Lock()
	h := ps.health[idx]
	h.record(err == nil, elapsed)

	if err == nil {
		qos := ps.qosLocked(idx, elapsed)
		ps.mu.Unlock()
		return qos, nil
	}

	shouldSwitch := h.consecFails >= ps.failThreshold
	qosBefore := ps.qosLocked(idx, elapsed)
	nextIdx := -1
	var nextCfg ProviderConfig
	var failureTime, detectedAt time.Time

	if shouldSwitch {
		h.online = false
		detectedAt = time.Now()
		if h.detectedAt == nil {
			h.detectedAt = &detectedAt
		}
		if h.failedAt != nil {
			failureTime = *h.failedAt
		} else {
			failureTime = detectedAt
		}
	}
	ps.mu.Unlock()

	// Phase 4: perform failover if threshold was exceeded.
	if shouldSwitch {
		mode := GetOrchestrationMode()
		netDelay := float64(GetNetworkDelayMs())

		if mode == "central" && ps.ah != nil {
			// Central reorchestration: ask Arrowhead for a new provider.
			// This call itself travels through the simulated network delay, adding
			// 2×networkDelay overhead (request + response) relative to local mode.
			nextIdx, nextCfg = ps.discoverViaCentral(idx)
		} else {
			// Local failover: use pre-configured list.
			nextIdx = ps.findFallback(idx)
			if nextIdx >= 0 {
				ps.mu.Lock()
				nextCfg = ps.configs[nextIdx]
				ps.activeIdx = nextIdx
				ps.mu.Unlock()
			}
		}

		if nextIdx < 0 {
			// No fallback available.
			return qosBefore, err
		}

		// Phase 5: retry on the fallback provider immediately.
		retryStart := time.Now()
		retryErr := DoRequest(method, nextCfg.URL+path, "", ps.cdtID, body, result)
		retryElapsed := float64(time.Since(retryStart)) / float64(time.Millisecond)

		ps.mu.Lock()
		nh := ps.health[nextIdx]
		nh.record(retryErr == nil, retryElapsed)
		qosAfter := ps.qosLocked(nextIdx, retryElapsed)

		// DecisionDelayMs = time from detection (threshold hit) to the switch call.
		// In local mode this is ~0ms; in central mode it includes the Arrowhead round-trip.
		decisionDelayMs := float64(retryStart.Sub(detectedAt)) / float64(time.Millisecond)

		ev := FailoverEvent{
			EventID:           fmt.Sprintf("fo-%d", retryStart.UnixNano()),
			CDTID:             ps.cdtID,
			Capability:        ps.capability,
			PrevProvider:      cfg.ID,
			NextProvider:      nextCfg.ID,
			FailureTime:       failureTime,
			DetectionTime:     detectedAt,
			SwitchTime:        retryStart,
			FailToSwitchMs:    float64(retryStart.Sub(failureTime)) / float64(time.Millisecond),
			DecisionDelayMs:   decisionDelayMs,
			OrchestrationMode: GetOrchestrationMode(),
			NetworkDelayMs:    netDelay,
			Reason:            fmt.Sprintf("%s: %d consecutive HTTP errors", cfg.ID, ps.health[idx].consecFails),
			QoSBefore:         qosBefore,
			QoSAfter:          qosAfter,
		}
		ps.events = append(ps.events, ev)
		if len(ps.events) > 20 {
			ps.events = ps.events[1:]
		}
		el := ps.eventLog
		ps.mu.Unlock()

		log.Printf("[%s] FAILOVER %s: %s → %s mode=%s netDelay=%.0fms decision=%.0fms total=%.0fms",
			ps.cdtID, ps.capability, cfg.ID, nextCfg.ID,
			ev.OrchestrationMode, netDelay, decisionDelayMs, ev.FailToSwitchMs)
		if el != nil {
			el.Write(ev)
		}
		return qosAfter, retryErr
	}

	return qosBefore, err
}

// discoverViaCentral asks Arrowhead for a provider of ps.capability.
// On success it updates ps.activeIdx and returns the index and config.
// On error it falls back to the local pre-configured list.
func (ps *ProviderSelector) discoverViaCentral(current int) (int, ProviderConfig) {
	// Simulate Arrowhead round-trip overhead: request + response = 2×networkDelay.
	if delay := GetNetworkDelayMs(); delay > 0 {
		time.Sleep(2 * time.Duration(delay) * time.Millisecond)
	}
	orch, err := ps.ah.Discover(ps.capability)
	if err != nil {
		log.Printf("[%s] Central discovery for %q failed: %v – falling back to local list",
			ps.cdtID, ps.capability, err)
		idx := ps.findFallback(current)
		if idx >= 0 {
			ps.mu.Lock()
			ps.activeIdx = idx
			cfg := ps.configs[idx]
			ps.mu.Unlock()
			return idx, cfg
		}
		return -1, ProviderConfig{}
	}

	// Match the returned endpoint against known configs.
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i, cfg := range ps.configs {
		if i != current && cfg.URL == orch.Endpoint {
			ps.activeIdx = i
			return i, cfg
		}
	}
	// Endpoint not in the pre-configured list: use first available fallback
	// and update its URL to point at the orchestration-provided endpoint.
	idx := ps.findFallbackLocked(current)
	if idx >= 0 {
		ps.configs[idx].URL = orch.Endpoint
		ps.activeIdx = idx
		return idx, ps.configs[idx]
	}
	return -1, ProviderConfig{}
}

// qosLocked computes QoSMetrics for provider idx using measured latency.
// Caller must hold ps.mu.
func (ps *ProviderSelector) qosLocked(idx int, measuredMs float64) QoSMetrics {
	cfg := ps.configs[idx]
	h := ps.health[idx]

	accuracy := cfg.NominalAccuracy
	if h.degraded && h.degradeFactor > 0 {
		accuracy = cfg.NominalAccuracy * h.degradeFactor
	}

	latency := measuredMs
	if latency <= 0 {
		latency = cfg.NominalLatencyMs
	}
	if h.degraded {
		latency += h.degradeExtraMs
	}

	freshness := 0.0
	if !h.lastGoodAt.IsZero() {
		freshness = float64(time.Since(h.lastGoodAt).Milliseconds())
	}

	rel := h.reliability()
	if rel > cfg.NominalReliability {
		rel = cfg.NominalReliability
	}

	return QoSMetrics{
		Accuracy:    accuracy,
		LatencyMs:   latency,
		Reliability: rel,
		FreshnessMs: freshness,
	}
}

// findFallback returns the index of the best available non-current provider.
// Prefers online providers; falls back to any other provider as last resort.
// Does NOT hold ps.mu – caller must not hold it.
func (ps *ProviderSelector) findFallback(current int) int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.findFallbackLocked(current)
}

// findFallbackLocked is findFallback but caller must already hold ps.mu.
func (ps *ProviderSelector) findFallbackLocked(current int) int {
	for i, h := range ps.health {
		if i != current && h.online {
			return i
		}
	}
	for i := range ps.configs {
		if i != current {
			return i
		}
	}
	return -1
}

// State returns a snapshot of all providers and QoS state (thread-safe).
func (ps *ProviderSelector) State() SourceQoS {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	active := ps.activeIdx
	providers := make([]ProviderState, len(ps.configs))
	for i, cfg := range ps.configs {
		h := ps.health[i]
		providers[i] = ProviderState{
			ID:       cfg.ID,
			URL:      cfg.URL,
			Primary:  cfg.Primary,
			Active:   i == active,
			Online:   h.online,
			Degraded: h.degraded,
			QoS:      ps.qosLocked(i, h.lastRespMs),
		}
	}
	events := make([]FailoverEvent, len(ps.events))
	copy(events, ps.events)

	return SourceQoS{
		Capability:      ps.capability,
		Active:          providers[active],
		Providers:       providers,
		Degraded:        active != 0 || ps.health[active].degraded,
		RecentFailovers: events,
		LastUpdated:     time.Now(),
	}
}

// ActiveProviderID returns the ID of the currently active provider (thread-safe).
func (ps *ProviderSelector) ActiveProviderID() string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.configs[ps.activeIdx].ID
}

// MarkFailed forces a provider into failed state; the next Do() will trigger failover.
func (ps *ProviderSelector) MarkFailed(providerID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i, cfg := range ps.configs {
		if cfg.ID == providerID {
			h := ps.health[i]
			h.online = false
			h.consecFails = ps.failThreshold + 1
			if h.failedAt == nil {
				t := time.Now()
				h.failedAt = &t
			}
			log.Printf("[%s] Provider %s force-marked FAILED", ps.cdtID, providerID)
			return
		}
	}
}

// MarkDegraded degrades a provider's reported QoS without triggering failover.
func (ps *ProviderSelector) MarkDegraded(providerID string, accuracyFactor, extraLatencyMs float64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i, cfg := range ps.configs {
		if cfg.ID == providerID {
			h := ps.health[i]
			h.degraded = true
			h.degradeFactor = accuracyFactor
			h.degradeExtraMs = extraLatencyMs
			log.Printf("[%s] Provider %s marked DEGRADED (accuracy×%.2f +%.0fms)",
				ps.cdtID, providerID, accuracyFactor, extraLatencyMs)
			return
		}
	}
}

// MarkRecovered clears failure/degradation state; re-activates primary if applicable.
func (ps *ProviderSelector) MarkRecovered(providerID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i, cfg := range ps.configs {
		if cfg.ID == providerID {
			h := ps.health[i]
			h.online = true
			h.degraded = false
			h.degradeFactor = 1.0
			h.degradeExtraMs = 0
			h.consecFails = 0
			h.failedAt = nil
			h.detectedAt = nil
			h.lastGoodAt = time.Now()
			log.Printf("[%s] Provider %s RECOVERED", ps.cdtID, providerID)
			if cfg.Primary && ps.activeIdx != i {
				log.Printf("[%s] Switching back to primary %s", ps.cdtID, providerID)
				ps.activeIdx = i
			}
			return
		}
	}
}

// BenchmarkDecision measures the time taken by the local vs. central failover
// decision path at a given simulated one-way network latency.
//
//   - Local path:   look up the fallback provider from the pre-configured list — sub-ms.
//   - Central path: simulate one Arrowhead round-trip (2×networkDelayMs sleep) then
//                   call Arrowhead Discover so any real orchestration overhead is included.
//
// Returns (localMs, centralMs). Both measurements are taken in isolation so they do
// not interfere with each other or with the normal Do() path.
func (ps *ProviderSelector) BenchmarkDecision(networkDelayMs int) (localMs, centralMs float64) {
	ps.mu.Lock()
	current := ps.activeIdx
	ps.mu.Unlock()

	// ── Local: pre-configured list lookup ───────────────────────────────────
	t0 := time.Now()
	ps.findFallback(current)
	localMs = float64(time.Since(t0)) / float64(time.Millisecond)

	// ── Central: Arrowhead round-trip ────────────────────────────────────────
	t1 := time.Now()
	if networkDelayMs > 0 {
		// 2× because the request travels to Arrowhead AND the response travels back.
		time.Sleep(2 * time.Duration(networkDelayMs) * time.Millisecond)
	}
	if ps.ah != nil {
		ps.ah.Discover(ps.capability) // result ignored; we measure the time, not the outcome
	}
	centralMs = float64(time.Since(t1)) / float64(time.Millisecond)

	return
}

// LatestFailoverEvent returns the most recent FailoverEvent, or nil if none recorded.
func (ps *ProviderSelector) LatestFailoverEvent() *FailoverEvent {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if len(ps.events) == 0 {
		return nil
	}
	ev := ps.events[len(ps.events)-1]
	return &ev
}
