package main

import "testing"

// ── PolicyStore CRUD ─────────────────────────────────────────────────────────

func TestPolicyStore_Add(t *testing.T) {
	s := NewPolicyStore()
	p, err := s.Add("sp1", "telemetry-rest", "", "consume", "Permit")
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if p.ID == "" {
		t.Error("Add: expected non-empty ID")
	}
	if p.Subject != "sp1" {
		t.Errorf("Add: subject = %q, want %q", p.Subject, "sp1")
	}
	if p.Resource != "telemetry-rest" {
		t.Errorf("Add: resource = %q, want %q", p.Resource, "telemetry-rest")
	}
	if p.Action != "consume" {
		t.Errorf("Add: action = %q, want %q", p.Action, "consume")
	}
	if p.Effect != "Permit" {
		t.Errorf("Add: effect = %q, want %q", p.Effect, "Permit")
	}
	if p.CreatedAt.IsZero() {
		t.Error("Add: CreatedAt is zero")
	}
}

func TestPolicyStore_Add_WithProvider(t *testing.T) {
	s := NewPolicyStore()
	p, err := s.Add("portal-cloud-ml", "telemetry", "robot-fleet-site-1", "orchestrate", "Permit")
	if err != nil {
		t.Fatalf("Add with provider: unexpected error: %v", err)
	}
	if p.Provider != "robot-fleet-site-1" {
		t.Errorf("Add with provider: provider = %q, want %q", p.Provider, "robot-fleet-site-1")
	}
	if p.Action != "orchestrate" {
		t.Errorf("Add with provider: action = %q, want orchestrate", p.Action)
	}
}

func TestPolicyStore_Add_DefaultEffect(t *testing.T) {
	s := NewPolicyStore()
	// empty effect → defaults to Permit
	p, err := s.Add("sp1", "svc", "", "consume", "")
	if err != nil {
		t.Fatalf("Add empty effect: unexpected error: %v", err)
	}
	if p.Effect != "Permit" {
		t.Errorf("Add empty effect: effect = %q, want Permit", p.Effect)
	}
}

func TestPolicyStore_Add_Validation(t *testing.T) {
	s := NewPolicyStore()
	cases := []struct{ subject, resource, provider, action, effect string }{
		{"", "svc", "", "consume", "Permit"},
		{"consumer", "", "", "consume", "Permit"},
		{"consumer", "svc", "", "", "Permit"},
		{"consumer", "svc", "", "consume", "Invalid"},
	}
	for _, c := range cases {
		if _, err := s.Add(c.subject, c.resource, c.provider, c.action, c.effect); err == nil {
			t.Errorf("Add(%q,%q,%q,%q,%q): expected validation error", c.subject, c.resource, c.provider, c.action, c.effect)
		}
	}
}

func TestPolicyStore_Get(t *testing.T) {
	s := NewPolicyStore()
	p, _ := s.Add("sp1", "telemetry-rest", "", "consume", "Permit")
	got, ok := s.Get(p.ID)
	if !ok {
		t.Fatal("Get: policy not found after Add")
	}
	if got.ID != p.ID {
		t.Errorf("Get: id = %q, want %q", got.ID, p.ID)
	}
}

func TestPolicyStore_Get_NotFound(t *testing.T) {
	s := NewPolicyStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get nonexistent: expected false")
	}
}

func TestPolicyStore_Delete_Existing(t *testing.T) {
	s := NewPolicyStore()
	p, _ := s.Add("sp1", "svc", "", "consume", "Permit")
	if !s.Delete(p.ID) {
		t.Error("Delete existing: expected true")
	}
	if _, ok := s.Get(p.ID); ok {
		t.Error("Delete: policy still found after deletion")
	}
}

func TestPolicyStore_Delete_NonExistent(t *testing.T) {
	s := NewPolicyStore()
	if s.Delete("nonexistent") {
		t.Error("Delete nonexistent: expected false")
	}
}

func TestPolicyStore_Delete_IdempotentAfterFirst(t *testing.T) {
	s := NewPolicyStore()
	p, _ := s.Add("sp1", "svc", "", "consume", "Permit")
	s.Delete(p.ID)
	if s.Delete(p.ID) {
		t.Error("Delete second time: expected false")
	}
}

func TestPolicyStore_GetAll_Empty(t *testing.T) {
	s := NewPolicyStore()
	all := s.GetAll()
	if len(all) != 0 {
		t.Errorf("GetAll empty store: len = %d, want 0", len(all))
	}
}

func TestPolicyStore_GetAll_Multiple(t *testing.T) {
	s := NewPolicyStore()
	s.Add("sp1", "telemetry-rest", "", "consume", "Permit")
	s.Add("sp2", "telemetry-rest", "", "consume", "Permit")
	all := s.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll: len = %d, want 2", len(all))
	}
}

func TestPolicyStore_Version_IncreasesOnAdd(t *testing.T) {
	s := NewPolicyStore()
	v0 := s.Version()
	s.Add("sp1", "svc", "", "consume", "Permit")
	if s.Version() <= v0 {
		t.Errorf("Version after Add: %d not > %d", s.Version(), v0)
	}
}

func TestPolicyStore_Version_IncreasesOnDelete(t *testing.T) {
	s := NewPolicyStore()
	p, _ := s.Add("sp1", "svc", "", "consume", "Permit")
	v1 := s.Version()
	s.Delete(p.ID)
	if s.Version() <= v1 {
		t.Errorf("Version after Delete: %d not > %d", s.Version(), v1)
	}
}

func TestPolicyStore_UniqueIDs(t *testing.T) {
	s := NewPolicyStore()
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		p, _ := s.Add("sp1", "svc", "", "consume", "Permit")
		if seen[p.ID] {
			t.Fatalf("Duplicate ID %q at iteration %d", p.ID, i)
		}
		seen[p.ID] = true
	}
}
