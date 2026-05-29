// Package model defines types for SimpleStoreServiceOrchestration.
package model

import orchmodel "arrowhead/core/internal/orchestration/model"

// StoreRule is a pre-configured peer-to-peer orchestration rule.
type StoreRule struct {
	ID                 string            `json:"id"`
	ConsumerSystemName string            `json:"consumerSystemName"`
	ServiceDefinition  string            `json:"serviceDefinition"`
	Provider           orchmodel.System  `json:"provider"`
	ServiceUri         string            `json:"serviceUri"`
	Interfaces         []string          `json:"interfaces"`
	Priority           int               `json:"priority,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// CreateRuleRequest is the body for POST mgmt/simple-store/create (and old /simplestore/rules alias).
type CreateRuleRequest struct {
	ConsumerSystemName string            `json:"consumerSystemName"`
	ServiceDefinition  string            `json:"serviceDefinition"`
	Provider           orchmodel.System  `json:"provider"`
	ServiceUri         string            `json:"serviceUri"`
	Interfaces         []string          `json:"interfaces"`
	Priority           int               `json:"priority,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// RulesResponse is returned by query endpoints.
type RulesResponse struct {
	Rules      []StoreRule `json:"rules"`
	Count      int         `json:"count"`
	TotalCount int         `json:"totalCount"`
}

// ModifyPrioritiesRequest maps rule UUID → new priority value.
type ModifyPrioritiesRequest struct {
	Priorities map[string]int `json:"priorities"`
}
