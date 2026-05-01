package main

import (
	"regexp"
	"testing"
)

func TestBuildPrefixPattern_single(t *testing.T) {
	got := buildPrefixPattern([]string{"telemetry"})
	want := "^telemetry\\."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPrefixPattern_multiple(t *testing.T) {
	got := buildPrefixPattern([]string{"telemetry", "sensors"})
	want := "^(sensors|telemetry)\\."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPrefixPattern_dedup(t *testing.T) {
	got := buildPrefixPattern([]string{"telemetry", "telemetry"})
	want := "^telemetry\\."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPrefixPattern_empty(t *testing.T) {
	got := buildPrefixPattern([]string{})
	if got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestBuildPrefixPattern_matchesRoutingKeys(t *testing.T) {
	pattern := buildPrefixPattern([]string{"telemetry"})
	re := regexp.MustCompile(pattern)

	matches := []string{
		"telemetry.robot-1",
		"telemetry.#",
		"telemetry.sensor.imu",
	}
	for _, key := range matches {
		if !re.MatchString(key) {
			t.Errorf("pattern %q should match %q but did not", pattern, key)
		}
	}

	nonMatches := []string{
		"other.x",
		"sensors.robot-1",
		"telemetryextra",
	}
	for _, key := range nonMatches {
		if re.MatchString(key) {
			t.Errorf("pattern %q should NOT match %q but did", pattern, key)
		}
	}
}

func TestBuildDesiredUsers_empty(t *testing.T) {
	result := BuildDesiredUsers(nil, "arrowhead")
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestBuildDesiredUsers_single(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ProviderSystemName: "robot-fleet", ServiceDefinition: "telemetry"},
	}
	result := BuildDesiredUsers(rules, "arrowhead")

	if len(result) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result))
	}
	tp, ok := result["consumer-1"]
	if !ok {
		t.Fatal("expected consumer-1 in result")
	}
	if tp.Exchange != "arrowhead" {
		t.Errorf("Exchange: got %q, want %q", tp.Exchange, "arrowhead")
	}
	if tp.Write != "" {
		t.Errorf("Write: got %q, want %q", tp.Write, "")
	}
	wantRead := "^telemetry\\."
	if tp.Read != wantRead {
		t.Errorf("Read: got %q, want %q", tp.Read, wantRead)
	}
}

func TestBuildDesiredUsers_merge(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ProviderSystemName: "robot-fleet", ServiceDefinition: "telemetry"},
		{ConsumerSystemName: "consumer-1", ProviderSystemName: "sensor-service", ServiceDefinition: "sensors"},
	}
	result := BuildDesiredUsers(rules, "arrowhead")

	if len(result) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result))
	}
	tp := result["consumer-1"]
	wantRead := "^(sensors|telemetry)\\."
	if tp.Read != wantRead {
		t.Errorf("Read: got %q, want %q", tp.Read, wantRead)
	}
}

func TestBuildDesiredUsers_twoConsumers(t *testing.T) {
	rules := []AuthRule{
		{ConsumerSystemName: "consumer-1", ProviderSystemName: "robot-fleet", ServiceDefinition: "telemetry"},
		{ConsumerSystemName: "consumer-2", ProviderSystemName: "robot-fleet", ServiceDefinition: "telemetry"},
	}
	result := BuildDesiredUsers(rules, "arrowhead")

	if len(result) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result))
	}
	for _, name := range []string{"consumer-1", "consumer-2"} {
		tp, ok := result[name]
		if !ok {
			t.Errorf("missing user %q", name)
			continue
		}
		wantRead := "^telemetry\\."
		if tp.Read != wantRead {
			t.Errorf("%s Read: got %q, want %q", name, tp.Read, wantRead)
		}
	}
}

func TestPublisherPermission(t *testing.T) {
	tp := PublisherPermission("arrowhead", []string{"telemetry"})
	if tp.Exchange != "arrowhead" {
		t.Errorf("Exchange: got %q, want %q", tp.Exchange, "arrowhead")
	}
	wantWrite := "^telemetry\\."
	if tp.Write != wantWrite {
		t.Errorf("Write: got %q, want %q", tp.Write, wantWrite)
	}
	if tp.Read != "" {
		t.Errorf("Read: got %q, want %q", tp.Read, "")
	}
}
