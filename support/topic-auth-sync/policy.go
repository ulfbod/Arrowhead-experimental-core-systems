package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// AuthRule mirrors the ConsumerAuth model.
type AuthRule struct {
	ID                 int64  `json:"id"`
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// LookupResponse mirrors the ConsumerAuth lookup endpoint response.
type LookupResponse struct {
	Rules []AuthRule `json:"rules"`
	Count int        `json:"count"`
}

// TopicPermission describes a RabbitMQ topic permission entry.
type TopicPermission struct {
	Exchange string
	Write    string
	Read     string
}

// DesiredUsers maps consumerSystemName → its TopicPermission.
type DesiredUsers map[string]TopicPermission

// BuildDesiredUsers groups rules by ConsumerSystemName, deduplicates
// ServiceDefinitions, and builds the merged read routing-key pattern.
func BuildDesiredUsers(rules []AuthRule, exchange string) DesiredUsers {
	// Group service definitions per consumer.
	byConsumer := make(map[string][]string)
	for _, r := range rules {
		byConsumer[r.ConsumerSystemName] = append(byConsumer[r.ConsumerSystemName], r.ServiceDefinition)
	}

	result := make(DesiredUsers)
	for consumer, services := range byConsumer {
		services = dedup(services)
		result[consumer] = TopicPermission{
			Exchange: exchange,
			Write:    "",
			Read:     buildPrefixPattern(services),
		}
	}
	return result
}

// PublisherPermission returns the topic permission for a publisher that may
// publish to all of the given service routing-key prefixes.
func PublisherPermission(exchange string, services []string) TopicPermission {
	return TopicPermission{
		Exchange: exchange,
		Write:    buildPrefixPattern(services),
		Read:     "",
	}
}

// buildPrefixPattern builds a regexp like `^telemetry\.` or `^(sensors|telemetry)\.`
// from a list of service definition names. Returns "" for an empty list.
func buildPrefixPattern(services []string) string {
	if len(services) == 0 {
		return ""
	}

	// Sort and deduplicate.
	services = dedup(services)

	// Quote each service name so special characters are escaped in the regexp.
	quoted := make([]string, len(services))
	for i, s := range services {
		quoted[i] = regexp.QuoteMeta(s)
	}

	if len(quoted) == 1 {
		return fmt.Sprintf("^%s\\.", quoted[0])
	}
	return fmt.Sprintf("^(%s)\\.", strings.Join(quoted, "|"))
}

// dedup returns a sorted, deduplicated copy of ss.
func dedup(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
