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
		ConsumerSystemName: "consumer-app",
		ProviderSystemName: "sensor-1",
		ServiceDefinition:  "temperature-service",
	}
}

// ---- Grant ----

func TestGrantValid(t *testing.T) {
	svc := newAuthService()
	rule, err := svc.Grant(validGrant())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if rule.ConsumerSystemName != "consumer-app" {
		t.Errorf("ConsumerSystemName = %q", rule.ConsumerSystemName)
	}
}

func TestGrantValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.GrantRequest)
	}{
		{"empty consumer", func(r *model.GrantRequest) { r.ConsumerSystemName = "" }},
		{"whitespace consumer", func(r *model.GrantRequest) { r.ConsumerSystemName = "  " }},
		{"empty provider", func(r *model.GrantRequest) { r.ProviderSystemName = "" }},
		{"empty service", func(r *model.GrantRequest) { r.ServiceDefinition = "" }},
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

func TestGrantSameConsumerDifferentServiceAllowed(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.ServiceDefinition = "humidity-service"
	_, err := svc.Grant(req2)
	if err != nil {
		t.Fatalf("different service should be allowed: %v", err)
	}
}

// ---- Revoke ----

func TestRevokeValid(t *testing.T) {
	svc := newAuthService()
	rule, _ := svc.Grant(validGrant())
	if err := svc.Revoke(rule.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := svc.Verify(model.VerifyRequest{
		ConsumerSystemName: rule.ConsumerSystemName,
		ProviderSystemName: rule.ProviderSystemName,
		ServiceDefinition:  rule.ServiceDefinition,
	})
	if resp.Authorized {
		t.Error("rule should no longer authorize after revoke")
	}
}

func TestRevokeNotFound(t *testing.T) {
	svc := newAuthService()
	if err := svc.Revoke(999); err == nil {
		t.Fatal("expected error for nonexistent rule")
	}
}

// ---- Lookup ----

func TestLookupNoFilter(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.ConsumerSystemName = "other-consumer"
	svc.Grant(req2)

	resp := svc.Lookup("", "", "")
	if resp.Count != 2 {
		t.Errorf("expected 2 rules, got %d", resp.Count)
	}
}

func TestLookupFilterByConsumer(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.ConsumerSystemName = "other"
	svc.Grant(req2)

	resp := svc.Lookup("consumer-app", "", "")
	if resp.Count != 1 {
		t.Errorf("expected 1, got %d", resp.Count)
	}
	if resp.Rules[0].ConsumerSystemName != "consumer-app" {
		t.Error("wrong rule returned")
	}
}

func TestLookupFilterByProvider(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.ConsumerSystemName = "c2"
	req2.ProviderSystemName = "other-provider"
	svc.Grant(req2)

	resp := svc.Lookup("", "sensor-1", "")
	if resp.Count != 1 {
		t.Errorf("expected 1, got %d", resp.Count)
	}
}

func TestLookupFilterByService(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	req2 := validGrant()
	req2.ConsumerSystemName = "c2"
	req2.ServiceDefinition = "pressure-service"
	svc.Grant(req2)

	resp := svc.Lookup("", "", "pressure-service")
	if resp.Count != 1 || resp.Rules[0].ServiceDefinition != "pressure-service" {
		t.Errorf("unexpected lookup result: %+v", resp)
	}
}

func TestLookupEmptyReturnsEmptySlice(t *testing.T) {
	svc := newAuthService()
	resp := svc.Lookup("", "", "")
	if resp.Rules == nil {
		t.Error("expected empty slice, not nil")
	}
	if resp.Count != 0 {
		t.Errorf("expected 0, got %d", resp.Count)
	}
}

// ---- Verify ----

func TestVerifyAuthorized(t *testing.T) {
	svc := newAuthService()
	rule, _ := svc.Grant(validGrant())

	resp := svc.Verify(model.VerifyRequest{
		ConsumerSystemName: "consumer-app",
		ProviderSystemName: "sensor-1",
		ServiceDefinition:  "temperature-service",
	})
	if !resp.Authorized {
		t.Error("expected authorized=true")
	}
	if resp.RuleID == nil || *resp.RuleID != rule.ID {
		t.Errorf("expected RuleID=%d, got %v", rule.ID, resp.RuleID)
	}
}

func TestVerifyUnauthorized(t *testing.T) {
	svc := newAuthService()
	resp := svc.Verify(model.VerifyRequest{
		ConsumerSystemName: "no-such-consumer",
		ProviderSystemName: "sensor-1",
		ServiceDefinition:  "temperature-service",
	})
	if resp.Authorized {
		t.Error("expected authorized=false")
	}
	if resp.RuleID != nil {
		t.Error("expected nil RuleID for unauthorized pair")
	}
}

// ---- GenerateToken ----

func TestGenerateTokenAuthorized(t *testing.T) {
	svc := newAuthService()
	svc.Grant(validGrant())
	resp, err := svc.GenerateToken(model.TokenRequest{
		ConsumerSystemName: "consumer-app",
		ProviderSystemName: "sensor-1",
		ServiceDefinition:  "temperature-service",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.ConsumerSystemName != "consumer-app" {
		t.Errorf("ConsumerSystemName = %q", resp.ConsumerSystemName)
	}
}

func TestGenerateTokenUnauthorized(t *testing.T) {
	svc := newAuthService()
	_, err := svc.GenerateToken(model.TokenRequest{
		ConsumerSystemName: "stranger",
		ProviderSystemName: "sensor-1",
		ServiceDefinition:  "temperature-service",
	})
	if err == nil {
		t.Fatal("expected error for unauthorized consumer")
	}
}
