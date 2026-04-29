package service_test

import (
	"testing"
	"time"

	"arrowhead/core/internal/authentication/model"
	"arrowhead/core/internal/authentication/repository"
	"arrowhead/core/internal/authentication/service"
)

func newAuthService(dur time.Duration) *service.AuthService {
	return service.NewAuthService(repository.NewMemoryRepository(), dur)
}

// ---- Login ----

func TestLoginValid(t *testing.T) {
	svc := newAuthService(time.Hour)
	resp, err := svc.Login(model.LoginRequest{SystemName: "sensor-1", Credentials: "any"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.SystemName != "sensor-1" {
		t.Errorf("SystemName = %q, want sensor-1", resp.SystemName)
	}
	if resp.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt must be in the future")
	}
}

func TestLoginEmptySystemName(t *testing.T) {
	svc := newAuthService(time.Hour)
	_, err := svc.Login(model.LoginRequest{SystemName: "", Credentials: "x"})
	if err == nil {
		t.Fatal("expected error for empty systemName")
	}
}

func TestLoginWhitespaceSystemName(t *testing.T) {
	svc := newAuthService(time.Hour)
	_, err := svc.Login(model.LoginRequest{SystemName: "   "})
	if err == nil {
		t.Fatal("expected error for whitespace systemName")
	}
}

func TestLoginCredentialsNotRequired(t *testing.T) {
	// Credentials field is accepted but not verified (see GAP_ANALYSIS G2).
	svc := newAuthService(time.Hour)
	_, err := svc.Login(model.LoginRequest{SystemName: "sys"})
	if err != nil {
		t.Fatalf("expected no error with empty credentials, got: %v", err)
	}
}

func TestLoginIssuesUniqueTokens(t *testing.T) {
	svc := newAuthService(time.Hour)
	r1, _ := svc.Login(model.LoginRequest{SystemName: "sys"})
	r2, _ := svc.Login(model.LoginRequest{SystemName: "sys"})
	if r1.Token == r2.Token {
		t.Error("expected two distinct tokens for two logins")
	}
}

// ---- Verify ----

func TestVerifyValidToken(t *testing.T) {
	svc := newAuthService(time.Hour)
	login, _ := svc.Login(model.LoginRequest{SystemName: "sys"})

	resp, err := svc.Verify(login.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Valid {
		t.Error("expected valid=true")
	}
	if resp.SystemName != "sys" {
		t.Errorf("SystemName = %q, want sys", resp.SystemName)
	}
}

func TestVerifyUnknownToken(t *testing.T) {
	svc := newAuthService(time.Hour)
	resp, err := svc.Verify("nonexistent-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Error("expected valid=false for unknown token")
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	// Negative duration places ExpiresAt in the past — token is immediately expired.
	svc := newAuthService(-time.Second)
	login, _ := svc.Login(model.LoginRequest{SystemName: "sys"})

	resp, err := svc.Verify(login.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Error("expected valid=false for expired token")
	}
}

func TestVerifyExpiredTokenIsDeleted(t *testing.T) {
	svc := newAuthService(-time.Second)
	login, _ := svc.Login(model.LoginRequest{SystemName: "sys"})
	svc.Verify(login.Token) // triggers lazy deletion

	// A second verify on the same token should also return invalid.
	resp, _ := svc.Verify(login.Token)
	if resp.Valid {
		t.Error("expected valid=false after expired token was lazily deleted")
	}
}

// ---- Logout ----

func TestLogoutValid(t *testing.T) {
	svc := newAuthService(time.Hour)
	login, _ := svc.Login(model.LoginRequest{SystemName: "sys"})

	if err := svc.Logout(login.Token); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, _ := svc.Verify(login.Token)
	if resp.Valid {
		t.Error("token should be invalid after logout")
	}
}

func TestLogoutUnknownToken(t *testing.T) {
	svc := newAuthService(time.Hour)
	err := svc.Logout("ghost-token")
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestLogoutIdempotentIsRejected(t *testing.T) {
	svc := newAuthService(time.Hour)
	login, _ := svc.Login(model.LoginRequest{SystemName: "sys"})
	svc.Logout(login.Token)

	// Second logout on the same token must fail.
	if err := svc.Logout(login.Token); err == nil {
		t.Error("expected error on second logout of same token")
	}
}
