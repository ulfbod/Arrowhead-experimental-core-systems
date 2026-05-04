package main

import (
	"regexp"
	"testing"
)

// ── hasAnyGrant ──────────────────────────────────────────────────────────────

func TestHasAnyGrant_consumerPresent(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if !hasAnyGrant(rules, "consumer-1") {
		t.Fatal("expected grant for consumer-1")
	}
}

func TestHasAnyGrant_consumerAbsent(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if hasAnyGrant(rules, "consumer-2") {
		t.Fatal("expected no grant for consumer-2")
	}
}

func TestHasAnyGrant_emptyRules(t *testing.T) {
	if hasAnyGrant(nil, "consumer-1") {
		t.Fatal("expected no grant with empty rules")
	}
}

// ── routingKeyAllowed ────────────────────────────────────────────────────────

func TestRoutingKeyAllowed_exactPublishKey(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if !routingKeyAllowed(rules, "consumer-1", "telemetry.robot-001") {
		t.Fatal("exact routing key should be allowed")
	}
}

func TestRoutingKeyAllowed_wildcardStar(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if !routingKeyAllowed(rules, "consumer-1", "telemetry.*") {
		t.Fatal("wildcard * bind key should be allowed")
	}
}

func TestRoutingKeyAllowed_wildcardHash(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if !routingKeyAllowed(rules, "consumer-1", "telemetry.#") {
		t.Fatal("wildcard # bind key should be allowed")
	}
}

func TestRoutingKeyAllowed_wrongService(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if routingKeyAllowed(rules, "consumer-1", "sensors.sensor-1") {
		t.Fatal("different service should be denied")
	}
}

func TestRoutingKeyAllowed_wrongConsumer(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if routingKeyAllowed(rules, "consumer-2", "telemetry.robot-001") {
		t.Fatal("different consumer should be denied")
	}
}

func TestRoutingKeyAllowed_exactServiceName(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	if !routingKeyAllowed(rules, "consumer-1", "telemetry") {
		t.Fatal("bare service name should be allowed")
	}
}

func TestRoutingKeyAllowed_multipleGrants(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "sensors"},
	}
	if !routingKeyAllowed(rules, "consumer-1", "sensors.sensor-1") {
		t.Fatal("sensor routing key should be allowed with sensors grant")
	}
	if !routingKeyAllowed(rules, "consumer-1", "telemetry.robot-001") {
		t.Fatal("telemetry routing key should be allowed with telemetry grant")
	}
}

func TestRoutingKeyAllowed_emptyRules(t *testing.T) {
	if routingKeyAllowed(nil, "consumer-1", "telemetry.robot-001") {
		t.Fatal("should be denied with empty rules")
	}
}

// ── buildPrefixPattern ───────────────────────────────────────────────────────

func TestBuildPrefixPattern_single(t *testing.T) {
	p := buildPrefixPattern([]string{"telemetry"})
	if p != `^telemetry\.` {
		t.Fatalf("unexpected pattern: %q", p)
	}
	if !regexp.MustCompile(p).MatchString("telemetry.robot-001") {
		t.Fatal("pattern should match telemetry.robot-001")
	}
}

func TestBuildPrefixPattern_multiple(t *testing.T) {
	p := buildPrefixPattern([]string{"sensors", "telemetry"})
	if p != `^(sensors|telemetry)\.` {
		t.Fatalf("unexpected pattern: %q", p)
	}
	re := regexp.MustCompile(p)
	if !re.MatchString("sensors.s1") {
		t.Fatal("pattern should match sensors.s1")
	}
	if !re.MatchString("telemetry.robot-001") {
		t.Fatal("pattern should match telemetry.robot-001")
	}
	if re.MatchString("other.x") {
		t.Fatal("pattern should not match other.x")
	}
}

func TestBuildPrefixPattern_dedup(t *testing.T) {
	p := buildPrefixPattern([]string{"telemetry", "telemetry"})
	if p != `^telemetry\.` {
		t.Fatalf("duplicate should be deduped: %q", p)
	}
}

func TestBuildPrefixPattern_empty(t *testing.T) {
	if buildPrefixPattern(nil) != "" {
		t.Fatal("empty services should return empty pattern")
	}
}

// ── BuildDesiredUsers ────────────────────────────────────────────────────────

func TestBuildDesiredUsers_empty(t *testing.T) {
	d := BuildDesiredUsers(nil, "arrowhead")
	if len(d) != 0 {
		t.Fatalf("expected empty desired users, got %v", d)
	}
}

func TestBuildDesiredUsers_singleRule(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}
	d := BuildDesiredUsers(rules, "arrowhead")
	tp, ok := d["consumer-1"]
	if !ok {
		t.Fatal("expected consumer-1 in desired users")
	}
	if tp.Exchange != "arrowhead" {
		t.Fatalf("unexpected exchange: %q", tp.Exchange)
	}
	if tp.Read != `^telemetry\.` {
		t.Fatalf("unexpected read pattern: %q", tp.Read)
	}
	if tp.Write != "" {
		t.Fatalf("consumer should have no write permission, got %q", tp.Write)
	}
}

func TestBuildDesiredUsers_mergedServices(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "sensors"},
	}
	d := BuildDesiredUsers(rules, "arrowhead")
	tp := d["consumer-1"]
	if tp.Read != `^(sensors|telemetry)\.` {
		t.Fatalf("unexpected merged pattern: %q", tp.Read)
	}
}
