package main

import "testing"

// PolicyStore tests are identical to experiment-10 — the store is unchanged.

func TestPolicyStore_Add(t *testing.T) {
	s := NewPolicyStore()
	p, err := s.Add("sp1", "telemetry-rest", "consume", "Permit")
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if p.ID == "" {
		t.Error("Add: expected non-empty ID")
	}
	if p.Subject != "sp1" {
		t.Errorf("Add: subject = %q, want sp1", p.Subject)
	}
	if p.Effect != "Permit" {
		t.Errorf("Add: effect = %q, want Permit", p.Effect)
	}
	if p.CreatedAt.IsZero() {
		t.Error("Add: CreatedAt is zero")
	}
}

func TestPolicyStore_Add_DefaultEffect(t *testing.T) {
	s := NewPolicyStore()
	p, err := s.Add("sp1", "svc", "consume", "")
	if err != nil {
		t.Fatalf("Add empty effect: %v", err)
	}
	if p.Effect != "Permit" {
		t.Errorf("default effect = %q, want Permit", p.Effect)
	}
}

func TestPolicyStore_Add_Validation(t *testing.T) {
	s := NewPolicyStore()
	cases := []struct{ subject, resource, action, effect string }{
		{"", "svc", "consume", "Permit"},
		{"consumer", "", "consume", "Permit"},
		{"consumer", "svc", "", "Permit"},
		{"consumer", "svc", "consume", "Invalid"},
	}
	for _, c := range cases {
		if _, err := s.Add(c.subject, c.resource, c.action, c.effect); err == nil {
			t.Errorf("Add(%q,%q,%q,%q): expected error", c.subject, c.resource, c.action, c.effect)
		}
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
	p, _ := s.Add("sp1", "svc", "consume", "Permit")
	if !s.Delete(p.ID) {
		t.Error("Delete existing: expected true")
	}
	if _, ok := s.Get(p.ID); ok {
		t.Error("Delete: still found after deletion")
	}
}

func TestPolicyStore_Delete_NonExistent(t *testing.T) {
	s := NewPolicyStore()
	if s.Delete("nonexistent") {
		t.Error("Delete nonexistent: expected false")
	}
}

func TestPolicyStore_Version_IncreasesOnAdd(t *testing.T) {
	s := NewPolicyStore()
	v0 := s.Version()
	s.Add("sp1", "svc", "consume", "Permit")
	if s.Version() <= v0 {
		t.Errorf("Version after Add: %d not > %d", s.Version(), v0)
	}
}

func TestPolicyStore_Version_IncreasesOnDelete(t *testing.T) {
	s := NewPolicyStore()
	p, _ := s.Add("sp1", "svc", "consume", "Permit")
	v1 := s.Version()
	s.Delete(p.ID)
	if s.Version() <= v1 {
		t.Errorf("Version after Delete: %d not > %d", s.Version(), v1)
	}
}

func TestPolicyStore_GetAll_Empty(t *testing.T) {
	s := NewPolicyStore()
	if len(s.GetAll()) != 0 {
		t.Error("GetAll empty: expected 0")
	}
}

func TestPolicyStore_UniqueIDs(t *testing.T) {
	s := NewPolicyStore()
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		p, _ := s.Add("sp1", "svc", "consume", "Permit")
		if seen[p.ID] {
			t.Fatalf("Duplicate ID %q at iteration %d", p.ID, i)
		}
		seen[p.ID] = true
	}
}
