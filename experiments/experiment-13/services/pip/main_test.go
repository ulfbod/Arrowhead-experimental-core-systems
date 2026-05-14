package main

import "testing"

func TestPIPEnvOr_Set(t *testing.T) {
	t.Setenv("PIP_TEST_KEY", "hello")
	if v := envOr("PIP_TEST_KEY", "default"); v != "hello" {
		t.Errorf("envOr set: got %q, want %q", v, "hello")
	}
}

func TestPIPEnvOr_Missing(t *testing.T) {
	if v := envOr("PIP_TEST_MISSING_XYZ", "default"); v != "default" {
		t.Errorf("envOr missing: got %q, want %q", v, "default")
	}
}

func TestPIPConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	cfg := configFromEnv()
	if cfg.port == "" {
		t.Error("configFromEnv: port should have a default")
	}
}

func TestPIPConfigFromEnv_Override(t *testing.T) {
	t.Setenv("PORT", "9999")
	cfg := configFromEnv()
	if cfg.port != "9999" {
		t.Errorf("port = %q, want 9999", cfg.port)
	}
}
