package common

import "time"

// QoSMetrics quantifies the quality of service delivered by one provider.
type QoSMetrics struct {
	Accuracy    float64 `json:"accuracy"`    // 0–1; 1.0 = nominal, < 1.0 = degraded
	LatencyMs   float64 `json:"latencyMs"`   // measured round-trip time, milliseconds
	Reliability float64 `json:"reliability"` // rolling success rate over last 10 requests, 0–1
	FreshnessMs float64 `json:"freshnessMs"` // milliseconds since last successful reading
}

// ProviderState describes one candidate provider and its live health status.
type ProviderState struct {
	ID       string     `json:"id"`
	URL      string     `json:"url"`
	Primary  bool       `json:"primary"`
	Active   bool       `json:"active"`
	Online   bool       `json:"online"`
	Degraded bool       `json:"degraded"`
	QoS      QoSMetrics `json:"qos"`
}

// FailoverEvent records a single provider-switch event with full timing data.
type FailoverEvent struct {
	EventID        string     `json:"eventId"`
	CDTID          string     `json:"cdtId"`
	Capability     string     `json:"capability"`
	PrevProvider   string     `json:"prevProvider"`
	NextProvider   string     `json:"nextProvider"`
	FailureTime    time.Time  `json:"failureTime"`    // when first HTTP error was seen
	DetectionTime  time.Time  `json:"detectionTime"`  // when failure threshold was reached
	SwitchTime     time.Time  `json:"switchTime"`     // when first call on fallback was issued
	FailToSwitchMs float64    `json:"failToSwitchMs"` // total delay: failure → switch, ms
	DecisionDelayMs float64   `json:"decisionDelayMs"` // detection → switch (excludes poll waiting); key metric
	OrchestrationMode string  `json:"orchestrationMode"` // "local" | "central" at time of event
	NetworkDelayMs float64    `json:"networkDelayMs"`    // simulated network delay at time of event
	Reason         string     `json:"reason"`
	QoSBefore      QoSMetrics `json:"qosBefore"`
	QoSAfter       QoSMetrics `json:"qosAfter"`
}

// SourceQoS is embedded in lower-cDT state responses so upper-cDTs can
// observe and propagate QoS state upward.
type SourceQoS struct {
	Capability      string          `json:"capability"`
	Active          ProviderState   `json:"active"`
	Providers       []ProviderState `json:"providers"`
	Degraded        bool            `json:"degraded"`        // true if on fallback or internally degraded
	RecentFailovers []FailoverEvent `json:"recentFailovers"` // most recent events, newest last
	LastUpdated     time.Time       `json:"lastUpdated"`
}
