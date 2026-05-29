package service_test

import (
	"regexp"
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
	resp, err := svc.Login(model.LoginRequest{SystemName: "sensor-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.SystemName != "sensor-1" {
		t.Errorf("SystemName = %q, want sensor-1", resp.SystemName)
	}
	if resp.ExpirationTime.Before(time.Now()) {
		t.Error("ExpirationTime must be in the future")
	}
}

func TestLoginEmptySystemName(t *testing.T) {
	svc := newAuthService(time.Hour)
	_, err := svc.Login(model.LoginRequest{SystemName: ""})
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
	if !resp.Verified {
		t.Error("expected verified=true")
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
	if resp.Verified {
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
	if resp.Verified {
		t.Error("expected valid=false for expired token")
	}
}

func TestVerifyExpiredTokenIsDeleted(t *testing.T) {
	svc := newAuthService(-time.Second)
	login, _ := svc.Login(model.LoginRequest{SystemName: "sys"})
	svc.Verify(login.Token) // triggers lazy deletion

	// A second verify on the same token should also return invalid.
	resp, _ := svc.Verify(login.Token)
	if resp.Verified {
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
	if resp.Verified {
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

// ---- Token format ----

func TestLoginTokenIsUUIDv4(t *testing.T) {
	svc := newAuthService(time.Hour)
	resp, err := svc.Login(model.LoginRequest{SystemName: "TestSystem"})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	uuidRe := regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)
	if !uuidRe.MatchString(resp.Token) {
		t.Errorf("token %q is not a UUID v4", resp.Token)
	}
}

func TestLoginTokensAreUnique(t *testing.T) {
	svc := newAuthService(time.Hour)
	r1, _ := svc.Login(model.LoginRequest{SystemName: "S"})
	r2, _ := svc.Login(model.LoginRequest{SystemName: "S"})
	if r1.Token == r2.Token {
		t.Error("two Login calls produced identical tokens")
	}
}

// ---- Cleanup goroutine ----

func TestDeleteExpiredCalledOnCleanup(t *testing.T) {
	repo := repository.NewMemoryRepository()
	// 1ms token duration so token expires almost immediately.
	// 10ms cleanup interval so cleanup fires well within the 50ms sleep.
	svc := service.NewAuthServiceWithCleanup(repo, time.Millisecond, 10*time.Millisecond)
	resp, _ := svc.Login(model.LoginRequest{SystemName: "expiry-test"})
	// Wait for cleanup to fire.
	time.Sleep(50 * time.Millisecond)
	// After cleanup, the token has been deleted from the repo.
	vr, _ := svc.Verify(resp.Token)
	if vr.Verified {
		t.Error("expected expired token to be gone after cleanup")
	}
}

// ---- ChangeCredentials ----

func TestChangeCredentialsActiveSession(t *testing.T) {
	svc := newAuthService(time.Hour)
	svc.Login(model.LoginRequest{SystemName: "sys-b"})
	if err := svc.ChangeCredentials("sys-b"); err != nil {
		t.Errorf("expected nil error for active session, got: %v", err)
	}
}

func TestChangeCredentialsNoSession(t *testing.T) {
	svc := newAuthService(time.Hour)
	if err := svc.ChangeCredentials("nobody"); err == nil {
		t.Error("expected error when no active session exists")
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
