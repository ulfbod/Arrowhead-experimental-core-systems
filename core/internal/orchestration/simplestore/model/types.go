// Package model defines types for SimpleStoreServiceOrchestration.
package model

import orchmodel "arrowhead/core/internal/orchestration/model"

// StoreRule is a pre-configured peer-to-peer orchestration rule.
type StoreRule struct {
	ID                 int64            `json:"id"`
	ConsumerSystemName string           `json:"consumerSystemName"`
	ServiceDefinition  string           `json:"serviceDefinition"`
	Provider           orchmodel.System `json:"provider"`
	ServiceUri         string           `json:"serviceUri"`
	Interfaces         []string         `json:"interfaces"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// CreateRuleRequest is the body for POST /orchestration/simplestore/rules.
type CreateRuleRequest struct {
	ConsumerSystemName string            `json:"consumerSystemName"`
	ServiceDefinition  string            `json:"serviceDefinition"`
	Provider           orchmodel.System  `json:"provider"`
	ServiceUri         string            `json:"serviceUri"`
	Interfaces         []string          `json:"interfaces"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// RulesResponse is returned by GET /orchestration/simplestore/rules.
type RulesResponse struct {
	Rules []StoreRule `json:"rules"`
	Count int         `json:"count"`
}
