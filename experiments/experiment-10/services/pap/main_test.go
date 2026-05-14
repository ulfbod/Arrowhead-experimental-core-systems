package main

import "testing"

func TestEnvOr_Set(t *testing.T) {
	t.Setenv("PAP_TEST_KEY", "hello")
	if v := envOr("PAP_TEST_KEY", "default"); v != "hello" {
		t.Errorf("envOr set: got %q, want %q", v, "hello")
	}
}

func TestEnvOr_Missing(t *testing.T) {
	if v := envOr("PAP_TEST_MISSING_XYZ", "default"); v != "default" {
		t.Errorf("envOr missing: got %q, want %q", v, "default")
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	// Unset overrides so defaults apply.
	t.Setenv("AUTHZFORCE_URL", "")
	t.Setenv("AUTHZFORCE_DOMAIN", "")
	t.Setenv("PORT", "")
	cfg := configFromEnv()
	if cfg.authzforceURL == "" {
		t.Error("configFromEnv: authzforceURL should have a default")
	}
	if cfg.domainExtID == "" {
		t.Error("configFromEnv: domainExtID should have a default")
	}
	if cfg.port == "" {
		t.Error("configFromEnv: port should have a default")
	}
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	t.Setenv("AUTHZFORCE_URL", "http://custom:8080/authzforce-ce")
	t.Setenv("AUTHZFORCE_DOMAIN", "test-domain")
	t.Setenv("PORT", "9999")
	cfg := configFromEnv()
	if cfg.authzforceURL != "http://custom:8080/authzforce-ce" {
		t.Errorf("authzforceURL = %q", cfg.authzforceURL)
	}
	if cfg.domainExtID != "test-domain" {
		t.Errorf("domainExtID = %q", cfg.domainExtID)
	}
	if cfg.port != "9999" {
		t.Errorf("port = %q", cfg.port)
	}
}
