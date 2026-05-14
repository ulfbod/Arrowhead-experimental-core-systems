package main

import "testing"

// ── GrantStore ────────────────────────────────────────────────────────────────

func TestGrantStore_InitialState(t *testing.T) {
	s := NewGrantStore()
	if s.Synced() {
		t.Error("new store: Synced should be false")
	}
	if s.Version() != 0 {
		t.Errorf("new store: Version = %d, want 0", s.Version())
	}
	if len(s.GetAll()) != 0 {
		t.Errorf("new store: GetAll len = %d, want 0", len(s.GetAll()))
	}
}

func TestGrantStore_Update_ChangesVersion(t *testing.T) {
	s := NewGrantStore()
	v0 := s.Version()
	grants := []Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc"},
	}
	changed := s.Update(grants)
	if !changed {
		t.Error("Update with new grants: expected changed=true")
	}
	if s.Version() <= v0 {
		t.Errorf("Version after Update: %d not > %d", s.Version(), v0)
	}
	if !s.Synced() {
		t.Error("Synced should be true after Update")
	}
}

func TestGrantStore_Update_NoChangeIfSameGrants(t *testing.T) {
	s := NewGrantStore()
	grants := []Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc"},
	}
	s.Update(grants)
	v1 := s.Version()
	changed := s.Update(grants) // same content
	if changed {
		t.Error("Update same grants: expected changed=false")
	}
	if s.Version() != v1 {
		t.Errorf("Version should not change on same grants: %d != %d", s.Version(), v1)
	}
}

func TestGrantStore_Update_ChangesVersionWhenContentDiffers(t *testing.T) {
	s := NewGrantStore()
	s.Update([]Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc"}})
	v1 := s.Version()
	changed := s.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc"},
		{ConsumerSystemName: "sp2", ServiceDefinition: "svc"},
	})
	if !changed {
		t.Error("Update with extra grant: expected changed=true")
	}
	if s.Version() <= v1 {
		t.Errorf("Version should increase: %d not > %d", s.Version(), v1)
	}
}

func TestGrantStore_Update_EmptyGrants(t *testing.T) {
	s := NewGrantStore()
	changed := s.Update(nil)
	// nil → still marks synced (first sync)
	if !changed {
		t.Error("First sync with empty grants: expected changed=true")
	}
	if !s.Synced() {
		t.Error("Synced should be true after first Update even with empty grants")
	}
}

func TestGrantStore_GetAll_ReturnsSnapshot(t *testing.T) {
	s := NewGrantStore()
	s.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc"},
		{ConsumerSystemName: "sp2", ServiceDefinition: "svc"},
	})
	all := s.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll: len = %d, want 2", len(all))
	}
}

func TestGrantStore_GetBySubjectResource(t *testing.T) {
	s := NewGrantStore()
	s.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"},
		{ConsumerSystemName: "sp2", ServiceDefinition: "svc-b"},
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc-b"},
	})
	// sp1 has two grants
	sp1 := s.GetBySubject("sp1")
	if len(sp1) != 2 {
		t.Errorf("GetBySubject sp1: len = %d, want 2", len(sp1))
	}
	// specific lookup
	ok := s.IsGranted("sp1", "svc-a")
	if !ok {
		t.Error("IsGranted sp1/svc-a: expected true")
	}
	ok = s.IsGranted("sp1", "svc-b")
	if !ok {
		t.Error("IsGranted sp1/svc-b: expected true")
	}
	ok = s.IsGranted("sp2", "svc-a")
	if ok {
		t.Error("IsGranted sp2/svc-a: expected false")
	}
}

func TestGrantStore_IsGranted_WhenNotSynced(t *testing.T) {
	s := NewGrantStore()
	if s.IsGranted("sp1", "svc") {
		t.Error("IsGranted before sync: expected false")
	}
}
