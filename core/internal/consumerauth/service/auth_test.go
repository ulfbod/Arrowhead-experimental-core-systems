package service_test

import (
	"testing"

	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/repository"
	"arrowhead/core/internal/consumerauth/service"
)

func newAuthService() *service.AuthService {
	return service.NewAuthService(repository.NewMemoryRepository())
}

func validGrant() model.GrantRequest {
	return model.GrantRequest{
		Provider:      "sensor-1",
		TargetType:    model.TargetServiceDef,
		Target:        "temperature-service",
		DefaultPolicy: model.PolicyDef{PolicyType: model.PolicyWhitelist, PolicyList: []string{"consumer-app"}},
	}
}

// ---- Grant ----

func TestGrantValid(t *testing.T) {
	svc := newAuthService()
	policy, err := svc.Grant(validGrant())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "PR|LOCAL|sensor-1|SERVICE_DEF|temperature-service"
	if policy.InstanceID != want {
		t.Errorf("InstanceID = %q, want %q", policy.InstanceID, want)
	}
	if policy.Provider != "sensor-1" {
		t.Errorf("Provider = %q", policy.Provider)
	}
}

func TestGrantValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.GrantRequest)
	}{
		{"empty provider", func(r *model.GrantRequest) { r.Provider = "" }},
		{"whitespace provider", func(r *model.GrantRequest) { r.Provider = "  " }},
		{"empty target", func(r *model.GrantRequest) { r.Target = "" }},
		{"empty targetType", func(r *model.GrantRequest) { r.TargetType = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validGrant()
			tc.mutate(&req)
			_, err := newAuthService().Grant(req)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGrantDuplicateReturnsError(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	_, err := svc.Grant(validGrant())
	if err == nil {
		t.Fatal("expected error for duplicate grant")
	}
	if err != service.ErrDuplicateRule {
		t.Errorf("expected ErrDuplicateRule, got %v", err)
	}
}

func TestGrantSameproviderDifferentTargetAllowed(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.Target = "humidity-service"
	_, err := svc.Grant(req2)
	if err != nil {
		t.Fatalf("different target should be allowed: %v", err)
	}
}

// ---- Revoke ----

func TestRevokeValid(t *testing.T) {
	svc := newAuthService()
	policy, _ := svc.Grant(validGrant())
	if err := svc.Revoke(policy.InstanceID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ok := svc.Verify(model.VerifyRequest{
		Consumer:   "consumer-app",
		Target:     "temperature-service",
		TargetType: model.TargetServiceDef,
	})
	if ok {
		t.Error("should not be authorized after revoke")
	}
}

func TestRevokeNotFound(t *testing.T) {
	svc := newAuthService()
	err := svc.Revoke("PR|LOCAL|nobody|SERVICE_DEF|svc")
	if err == nil {
		t.Fatal("expected error for nonexistent policy")
	}
}

// ---- Lookup ----

func TestLookupByTargetType(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.Target = "humidity-service"
	svc.Grant(req2)

	resp := svc.Lookup(model.LookupRequest{TargetType: model.TargetServiceDef})
	if resp.Count != 2 {
		t.Errorf("expected 2 policies, got %d", resp.Count)
	}
}

func TestLookupByTargetName(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.Target = "humidity-service"
	svc.Grant(req2)

	resp := svc.Lookup(model.LookupRequest{TargetNames: []string{"temperature-service"}})
	if resp.Count != 1 {
		t.Errorf("expected 1, got %d", resp.Count)
	}
	if resp.Policies[0].Target != "temperature-service" {
		t.Error("wrong policy returned")
	}
}

func TestLookupEmptyReturnsEmptySlice(t *testing.T) {
	svc := newAuthService()
	resp := svc.Lookup(model.LookupRequest{})
	if resp.Policies == nil {
		t.Error("expected empty slice, not nil")
	}
	if resp.Count != 0 {
		t.Errorf("expected 0, got %d", resp.Count)
	}
}

// ---- Verify ----

func TestVerifyAuthorizedWhitelist(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	ok := svc.Verify(model.VerifyRequest{
		Consumer:   "consumer-app",
		Target:     "temperature-service",
		TargetType: model.TargetServiceDef,
	})
	if !ok {
		t.Error("expected authorized=true")
	}
}

func TestVerifyAuthorizedAll(t *testing.T) {
	svc := newAuthService()
	svc.Grant(model.GrantRequest{
		Provider:      "prov",
		TargetType:    model.TargetServiceDef,
		Target:        "svc",
		DefaultPolicy: model.PolicyDef{PolicyType: model.PolicyAll},
	})
	ok := svc.Verify(model.VerifyRequest{Consumer: "anyone", Target: "svc", TargetType: model.TargetServiceDef})
	if !ok {
		t.Error("expected authorized=true for ALL policy")
	}
}

func TestVerifyUnauthorizedNotInWhitelist(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	ok := svc.Verify(model.VerifyRequest{
		Consumer:   "no-such-consumer",
		Target:     "temperature-service",
		TargetType: model.TargetServiceDef,
	})
	if ok {
		t.Error("expected authorized=false")
	}
}

func TestVerifyBlacklist(t *testing.T) {
	svc := newAuthService()
	svc.Grant(model.GrantRequest{
		Provider:      "prov",
		TargetType:    model.TargetServiceDef,
		Target:        "svc",
		DefaultPolicy: model.PolicyDef{PolicyType: model.PolicyBlacklist, PolicyList: []string{"bad-actor"}},
	})
	if svc.Verify(model.VerifyRequest{Consumer: "bad-actor", Target: "svc", TargetType: model.TargetServiceDef}) {
		t.Error("blacklisted consumer should not be authorized")
	}
	if !svc.Verify(model.VerifyRequest{Consumer: "good-actor", Target: "svc", TargetType: model.TargetServiceDef}) {
		t.Error("non-blacklisted consumer should be authorized")
	}
}

func TestVerifyWithProvider(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant()) // sensor-1, WHITELIST: [consumer-app]
	// sensor-2 has no policy → consumer-app should NOT be authorized for sensor-2
	if svc.Verify(model.VerifyRequest{
		Consumer: "consumer-app", Provider: "sensor-2",
		Target: "temperature-service", TargetType: model.TargetServiceDef,
	}) {
		t.Error("expected false: no policy for sensor-2")
	}
	// sensor-1 has policy → consumer-app should be authorized
	if !svc.Verify(model.VerifyRequest{
		Consumer: "consumer-app", Provider: "sensor-1",
		Target: "temperature-service", TargetType: model.TargetServiceDef,
	}) {
		t.Error("expected true: consumer-app is in whitelist for sensor-1")
	}
}

// ─── Step 34 (G23): Token variants ───────────────────────────────────────────

func TestUsageLimitedTokenGenerated(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant:  model.TokenVariantUsageLimited,
		Provider:      "provider-1",
		TargetType:    model.TargetServiceDef,
		Target:        "svc",
		MaxUsageCount: 3,
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("generate USAGE_LIMITED_TOKEN: %v", err)
	}
	if desc.Token == "" {
		t.Error("expected non-empty token")
	}
	if desc.TokenType != model.TokenVariantUsageLimited {
		t.Errorf("token type = %q, want %q", desc.TokenType, model.TokenVariantUsageLimited)
	}
}

func TestUsageLimitedTokenDecrementsOnVerify(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant:  model.TokenVariantUsageLimited,
		Provider:      "provider-1",
		TargetType:    model.TargetServiceDef,
		Target:        "svc",
		MaxUsageCount: 2,
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	token := desc.Token
	// First two verifications should succeed
	for i := 0; i < 2; i++ {
		_, ok := svc.VerifyAuthToken(token)
		if !ok {
			t.Errorf("verify #%d: want ok=true", i+1)
		}
	}
	// Third should fail
	_, ok := svc.VerifyAuthToken(token)
	if ok {
		t.Error("third verify: want ok=false (exhausted)")
	}
}

func TestUsageLimitedTokenExpiredAfterMaxUsage(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant:  model.TokenVariantUsageLimited,
		Provider:      "provider-1",
		TargetType:    model.TargetServiceDef,
		Target:        "svc",
		MaxUsageCount: 1,
	}
	desc, _ := svc.GenerateAuthToken(req)
	token := desc.Token
	svc.VerifyAuthToken(token) // exhausts
	// After exhaustion, verify still returns false
	_, ok := svc.VerifyAuthToken(token)
	if ok {
		t.Error("after exhaustion: want ok=false")
	}
}

func TestBase64SelfContainedTokenGenerated(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant: model.TokenVariantBase64SelfContained,
		Provider:     "provider-1",
		TargetType:   model.TargetServiceDef,
		Target:       "svc",
		Consumer:     "consumer-1",
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("generate BASE64_SELF_CONTAINED: %v", err)
	}
	if desc.Token == "" {
		t.Error("expected non-empty token")
	}
	if desc.TokenType != model.TokenVariantBase64SelfContained {
		t.Errorf("token type = %q", desc.TokenType)
	}
}

func TestBase64SelfContainedTokenVerifiable(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant: model.TokenVariantBase64SelfContained,
		Provider:     "provider-1",
		TargetType:   model.TargetServiceDef,
		Target:       "svc",
		Consumer:     "consumer-1",
	}
	desc, _ := svc.GenerateAuthToken(req)
	// Create a new service (different instance) — token must still verify
	svc2 := newAuthService()
	resp, ok := svc2.VerifyAuthToken(desc.Token)
	if !ok {
		t.Fatal("BASE64_SELF_CONTAINED token should verify without server state")
	}
	if resp.Consumer != "consumer-1" {
		t.Errorf("consumer = %q, want consumer-1", resp.Consumer)
	}
}

func TestJWTVariantReturns501(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant: "JWT_TOKEN",
		Provider:     "provider-1",
		TargetType:   model.TargetServiceDef,
		Target:       "svc",
	}
	_, err := svc.GenerateAuthToken(req)
	if err == nil {
		t.Fatal("expected error for unsupported JWT variant")
	}
}

// ─── G57 — Token descriptor fields (usageLimit, expiresAt optionality) ───────

func TestGenerateAuthTokenUsageLimitedHasUsageLimitNoExpiresAt(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant:  model.TokenVariantUsageLimited,
		Provider:      "Provider1",
		TargetType:    model.TargetServiceDef,
		Target:        "tempService",
		MaxUsageCount: 5,
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.UsageLimit == nil {
		t.Error("UsageLimit should not be nil for USAGE_LIMITED_TOKEN")
	} else if *desc.UsageLimit != 5 {
		t.Errorf("UsageLimit = %d, want 5", *desc.UsageLimit)
	}
	if desc.ExpiresAt != "" {
		t.Errorf("ExpiresAt should be empty for USAGE_LIMITED_TOKEN, got %q", desc.ExpiresAt)
	}
}

func TestGenerateAuthTokenTimeLimitedHasExpiresAtNoUsageLimit(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant: model.TokenVariantTimeLimited,
		Provider:     "Provider1",
		TargetType:   model.TargetServiceDef,
		Target:       "tempService",
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.ExpiresAt == "" {
		t.Error("ExpiresAt should be set for TIME_LIMITED_TOKEN")
	}
	if desc.UsageLimit != nil {
		t.Errorf("UsageLimit should be nil for TIME_LIMITED_TOKEN, got %d", *desc.UsageLimit)
	}
}

func TestGenerateAuthTokenBase64SelfContainedOmitsExpiresAtAndUsageLimit(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant: model.TokenVariantBase64SelfContained,
		Provider:     "Provider1",
		TargetType:   model.TargetServiceDef,
		Target:       "tempService",
		Consumer:     "Consumer1",
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// BASE64_SELF_CONTAINED embeds expiry in the payload — descriptor should not carry expiresAt.
	if desc.ExpiresAt != "" {
		t.Errorf("ExpiresAt should be empty for BASE64_SELF_CONTAINED, got %q", desc.ExpiresAt)
	}
	if desc.UsageLimit != nil {
		t.Errorf("UsageLimit should be nil for BASE64_SELF_CONTAINED, got %d", *desc.UsageLimit)
	}
}

func TestGenerateAuthTokenUsageLimitedDefaultsToOne(t *testing.T) {
	svc := newAuthService()
	req := model.TokenGenerateRequest{
		TokenVariant:  model.TokenVariantUsageLimited,
		Provider:      "Provider1",
		TargetType:    model.TargetServiceDef,
		Target:        "tempService",
		MaxUsageCount: 0, // should default to 1
	}
	desc, err := svc.GenerateAuthToken(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.UsageLimit == nil || *desc.UsageLimit != 1 {
		t.Errorf("UsageLimit should default to 1, got %v", desc.UsageLimit)
	}
}
