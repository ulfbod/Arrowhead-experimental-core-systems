// Package model defines types for FlexibleStoreServiceOrchestration.
//
// Design assumption: AH5 docs are "coming soon" for FlexibleStore.
// This implementation extends SimpleStore with a priority field and
// metadata-based rule matching. See GAP_ANALYSIS.md for the full rationale.
package model

import orchmodel "arrowhead/core/internal/orchestration/model"

// FlexibleRule is a priority-ordered orchestration rule with optional metadata filter.
type FlexibleRule struct {
	ID                 int64             `json:"id"`
	ConsumerSystemName string            `json:"consumerSystemName"`
	ServiceDefinition  string            `json:"serviceDefinition"`
	Provider           orchmodel.System  `json:"provider"`
	ServiceUri         string            `json:"serviceUri"`
	Interfaces         []string          `json:"interfaces"`
	// Priority: lower value = higher priority (1 = highest).
	Priority           int               `json:"priority"`
	// MetadataFilter: rule only matches requests whose requestedService.metadata
	// contains ALL key-value pairs listed here.
	MetadataFilter     map[string]string `json:"metadataFilter,omitempty"`
}

// CreateFlexibleRuleRequest is the body for POST /orchestration/flexiblestore/rules.
type CreateFlexibleRuleRequest struct {
	ConsumerSystemName string            `json:"consumerSystemName"`
	ServiceDefinition  string            `json:"serviceDefinition"`
	Provider           orchmodel.System  `json:"provider"`
	ServiceUri         string            `json:"serviceUri"`
	Interfaces         []string          `json:"interfaces"`
	Priority           int               `json:"priority"`
	MetadataFilter     map[string]string `json:"metadataFilter,omitempty"`
}

// RulesResponse is returned by GET /orchestration/flexiblestore/rules.
type RulesResponse struct {
	Rules []FlexibleRule `json:"rules"`
	Count int            `json:"count"`
}
