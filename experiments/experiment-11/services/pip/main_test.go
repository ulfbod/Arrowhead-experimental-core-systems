package main

import "testing"

func TestPIPEnvOr_Set(t *testing.T) {
	t.Setenv("PIP11_TEST_KEY", "hello")
	if v := envOr("PIP11_TEST_KEY", "default"); v != "hello" {
		t.Errorf("envOr set: got %q, want %q", v, "hello")
	}
}

func TestPIPEnvOr_Missing(t *testing.T) {
	if v := envOr("PIP11_TEST_MISSING_XYZ", "default"); v != "default" {
		t.Errorf("envOr missing: got %q, want %q", v, "default")
	}
}

func TestPIPConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("CONSUMERAUTH_URL", "")
	t.Setenv("SYNC_INTERVAL", "")
	cfg := configFromEnv()
	if cfg.port == "" {
		t.Error("configFromEnv: port should have a default")
	}
	if cfg.consumerAuthURL == "" {
		t.Error("configFromEnv: consumerAuthURL should have a default")
	}
	if cfg.syncInterval == 0 {
		t.Error("configFromEnv: syncInterval should have a default")
	}
}

func TestPIPConfigFromEnv_Override(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("CONSUMERAUTH_URL", "http://ca:8082")
	t.Setenv("SYNC_INTERVAL", "5s")
	cfg := configFromEnv()
	if cfg.port != "9999" {
		t.Errorf("port = %q, want 9999", cfg.port)
	}
	if cfg.consumerAuthURL != "http://ca:8082" {
		t.Errorf("consumerAuthURL = %q", cfg.consumerAuthURL)
	}
}
