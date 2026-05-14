package main

import "testing"

// ── SubjectStore CRUD ─────────────────────────────────────────────────────────

func TestSubjectStore_Register(t *testing.T) {
	s := NewSubjectStore()
	sub, err := s.Register("sp1", "sy", true)
	if err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}
	if sub.Name != "sp1" {
		t.Errorf("Register: name = %q, want sp1", sub.Name)
	}
	if sub.CertLevel != "sy" {
		t.Errorf("Register: certLevel = %q, want sy", sub.CertLevel)
	}
	if !sub.Valid {
		t.Error("Register: valid = false, want true")
	}
	if sub.RegisteredAt.IsZero() {
		t.Error("Register: RegisteredAt is zero")
	}
}

func TestSubjectStore_Register_InvalidLevel(t *testing.T) {
	s := NewSubjectStore()
	cases := []string{"", "xx", "root", "admin"}
	for _, level := range cases {
		if _, err := s.Register("sp1", level, true); err == nil {
			t.Errorf("Register level %q: expected error, got nil", level)
		}
	}
}

func TestSubjectStore_Register_EmptyName(t *testing.T) {
	s := NewSubjectStore()
	if _, err := s.Register("", "sy", true); err == nil {
		t.Error("Register empty name: expected error, got nil")
	}
}

func TestSubjectStore_Register_ValidLevels(t *testing.T) {
	s := NewSubjectStore()
	for _, level := range []string{"lo", "on", "de", "sy"} {
		if _, err := s.Register("sys-"+level, level, true); err != nil {
			t.Errorf("Register level %q: unexpected error: %v", level, err)
		}
	}
}

func TestSubjectStore_Get(t *testing.T) {
	s := NewSubjectStore()
	s.Register("sp1", "sy", true)
	sub, ok := s.Get("sp1")
	if !ok {
		t.Fatal("Get: subject not found after Register")
	}
	if sub.Name != "sp1" {
		t.Errorf("Get: name = %q, want sp1", sub.Name)
	}
}

func TestSubjectStore_Get_NotFound(t *testing.T) {
	s := NewSubjectStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get nonexistent: expected false")
	}
}

func TestSubjectStore_Register_Upsert(t *testing.T) {
	s := NewSubjectStore()
	s.Register("sp1", "on", true)
	// Re-register with updated cert level
	sub, err := s.Register("sp1", "sy", false)
	if err != nil {
		t.Fatalf("Register upsert: unexpected error: %v", err)
	}
	if sub.CertLevel != "sy" {
		t.Errorf("Upsert: certLevel = %q, want sy", sub.CertLevel)
	}
	if sub.Valid {
		t.Error("Upsert: valid = true, want false")
	}
}

func TestSubjectStore_Delete_Existing(t *testing.T) {
	s := NewSubjectStore()
	s.Register("sp1", "sy", true)
	if !s.Delete("sp1") {
		t.Error("Delete existing: expected true")
	}
	if _, ok := s.Get("sp1"); ok {
		t.Error("Delete: subject still found after deletion")
	}
}

func TestSubjectStore_Delete_NonExistent(t *testing.T) {
	s := NewSubjectStore()
	if s.Delete("nonexistent") {
		t.Error("Delete nonexistent: expected false")
	}
}

func TestSubjectStore_GetAll_Empty(t *testing.T) {
	s := NewSubjectStore()
	if len(s.GetAll()) != 0 {
		t.Error("GetAll empty: expected 0")
	}
}

func TestSubjectStore_GetAll_Multiple(t *testing.T) {
	s := NewSubjectStore()
	s.Register("sp1", "sy", true)
	s.Register("sp2", "sy", true)
	if len(s.GetAll()) != 2 {
		t.Errorf("GetAll: len = %d, want 2", len(s.GetAll()))
	}
}
