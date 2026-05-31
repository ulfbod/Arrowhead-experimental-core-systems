// Package model defines types for the Device QoS Evaluator core system.
package model

// MeasurementRequest is the body for POST /deviceqosevaluator/quality-evaluation/measure.
type MeasurementRequest struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

// QoSRecord is a single QoS measurement result.
type QoSRecord struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Port       string `json:"port"`
	LatencyMs  int64  `json:"latencyMs"`
	MeasuredAt string `json:"measuredAt"`
	Reachable  bool   `json:"reachable"`
}

// QueryRequest filters for POST /deviceqosevaluator/quality-evaluation/mgmt/query.
type QueryRequest struct {
	Host       string       `json:"host,omitempty"`
	Port       string       `json:"port,omitempty"`
	From       string       `json:"from,omitempty"`
	To         string       `json:"to,omitempty"`
	Pagination *PageRequest `json:"pagination,omitempty"`
}

// PageRequest carries pagination parameters.
type PageRequest struct {
	Page int `json:"page"`
	Size int `json:"size"`
}

// QueryResponse is returned by POST /deviceqosevaluator/quality-evaluation/mgmt/query.
type QueryResponse struct {
	Records    []*QoSRecord `json:"records"`
	Count      int          `json:"count"`
	TotalCount int          `json:"totalCount"`
}
