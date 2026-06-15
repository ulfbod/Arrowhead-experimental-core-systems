package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func newTestCA(t *testing.T) *ProfileCA {
	t.Helper()
	ca, err := NewProfileCA(24 * time.Hour, "")
	if err != nil {
		t.Fatalf("NewProfileCA: %v", err)
	}
	return ca
}

func parseCert(t *testing.T, pemStr string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

func TestNewProfileCA(t *testing.T) {
	ca := newTestCA(t)
	if ca.CACertPEM() == "" {
		t.Error("CACertPEM should not be empty")
	}
	if ca.CACert().Subject.CommonName != "Arrowhead Local Cloud CA" {
		t.Errorf("unexpected CN: %s", ca.CACert().Subject.CommonName)
	}
	if len(ca.CACert().Subject.OrganizationalUnit) == 0 || ca.CACert().Subject.OrganizationalUnit[0] != "lo" {
		t.Errorf("CA cert should have OU=lo, got %v", ca.CACert().Subject.OrganizationalUnit)
	}
}

func TestIssueOnboardingCert(t *testing.T) {
	ca := newTestCA(t)
	certPEM, keyPEM, err := ca.IssueOnboardingCert("my-device")
	if err != nil {
		t.Fatalf("IssueOnboardingCert: %v", err)
	}
	if certPEM == "" || keyPEM == "" {
		t.Error("cert or key should not be empty")
	}
	cert := parseCert(t, certPEM)
	if cert.Subject.CommonName != "my-device" {
		t.Errorf("expected CN=my-device, got %s", cert.Subject.CommonName)
	}
	if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != "on" {
		t.Errorf("expected OU=on, got %v", cert.Subject.OrganizationalUnit)
	}
	// Verify cert chains to CA
	pool := x509.NewCertPool()
	pool.AddCert(ca.CACert())
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Errorf("cert should verify against CA: %v", err)
	}
}

func TestIssueDeviceCert_RequiresOnboardingCert(t *testing.T) {
	ca := newTestCA(t)
	onCertPEM, onKeyPEM, _ := ca.IssueOnboardingCert("my-device")
	onCert := parseCert(t, onCertPEM)

	certPEM, keyPEM, err := ca.IssueDeviceCert("my-device", onCert)
	if err != nil {
		t.Fatalf("IssueDeviceCert: %v", err)
	}
	// Verify pair is usable
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		t.Errorf("key pair invalid: %v", err)
	}
	cert := parseCert(t, certPEM)
	if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != "de" {
		t.Errorf("expected OU=de, got %v", cert.Subject.OrganizationalUnit)
	}
	_ = onKeyPEM // referenced to avoid unused warning
}

func TestIssueDeviceCert_RejectsSystemCert(t *testing.T) {
	ca := newTestCA(t)
	// Issue a system cert directly (infra path) and try to use it to get a device cert
	syCertPEM, _, _ := ca.IssueInfraCert("bad-actor")
	syCert := parseCert(t, syCertPEM)

	_, _, err := ca.IssueDeviceCert("bad-actor", syCert)
	if err == nil {
		t.Error("IssueDeviceCert should reject system cert as issuer")
	}
}

func TestIssueSystemCert_RequiresDeviceCert(t *testing.T) {
	ca := newTestCA(t)
	onCertPEM, _, _ := ca.IssueOnboardingCert("my-system")
	onCert := parseCert(t, onCertPEM)
	deCertPEM, deKeyPEM, _ := ca.IssueDeviceCert("my-system", onCert)
	deCert := parseCert(t, deCertPEM)

	certPEM, keyPEM, err := ca.IssueSystemCert("my-system", deCert)
	if err != nil {
		t.Fatalf("IssueSystemCert: %v", err)
	}
	cert := parseCert(t, certPEM)
	if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != "sy" {
		t.Errorf("expected OU=sy, got %v", cert.Subject.OrganizationalUnit)
	}
	_ = deKeyPEM
	_ = keyPEM
}

func TestIssueSystemCert_RejectsOnboardingCert(t *testing.T) {
	ca := newTestCA(t)
	onCertPEM, _, _ := ca.IssueOnboardingCert("shortcut")
	onCert := parseCert(t, onCertPEM)

	_, _, err := ca.IssueSystemCert("shortcut", onCert)
	if err == nil {
		t.Error("IssueSystemCert should reject onboarding cert as issuer (must use device cert)")
	}
}

func TestIssueDeviceCert_RejectsCertFromDifferentCA(t *testing.T) {
	ca1 := newTestCA(t)
	ca2 := newTestCA(t)
	onCertPEM, _, _ := ca1.IssueOnboardingCert("impersonator")
	onCert := parseCert(t, onCertPEM)

	_, _, err := ca2.IssueDeviceCert("impersonator", onCert)
	if err == nil {
		t.Error("IssueDeviceCert should reject cert from different CA")
	}
}

func TestIssueInfraCert(t *testing.T) {
	ca := newTestCA(t)
	certPEM, keyPEM, err := ca.IssueInfraCert("kafka")
	if err != nil {
		t.Fatalf("IssueInfraCert: %v", err)
	}
	cert := parseCert(t, certPEM)
	if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != "sy" {
		t.Errorf("infra cert should have OU=sy, got %v", cert.Subject.OrganizationalUnit)
	}
	_ = keyPEM
}

func TestTLSCert(t *testing.T) {
	ca := newTestCA(t)
	tlsCert, err := ca.TLSCert()
	if err != nil {
		t.Fatalf("TLSCert: %v", err)
	}
	if len(tlsCert.Certificate) == 0 {
		t.Error("TLSCert should have at least one certificate")
	}
}

func TestIssueOnboardingCert_EmptySystemName(t *testing.T) {
	ca := newTestCA(t)
	_, _, err := ca.IssueOnboardingCert("")
	if err == nil {
		t.Error("should reject empty systemName")
	}
}

// --- Experiment-13 additions: cert registry, revocation, subscribe ---

// TestCertRegistry_IssuedCertRecorded verifies that after issuing a cert,
// GetAll returns a record for it.
func TestCertRegistry_IssuedCertRecorded(t *testing.T) {
	ca := newTestCA(t)
	_, _, err := ca.IssueOnboardingCert("device-alpha")
	if err != nil {
		t.Fatalf("IssueOnboardingCert: %v", err)
	}
	records := ca.GetAll()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].CN != "device-alpha" {
		t.Errorf("expected CN=device-alpha, got %s", records[0].CN)
	}
	if records[0].OU != "on" {
		t.Errorf("expected OU=on, got %s", records[0].OU)
	}
	if records[0].Revoked {
		t.Error("newly issued cert should not be revoked")
	}
	if records[0].IssuedAt.IsZero() {
		t.Error("IssuedAt should be set")
	}
	if records[0].ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}
}

// TestCertRegistry_MultipleRecords verifies multiple issuances are all recorded.
func TestCertRegistry_MultipleRecords(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueOnboardingCert("alpha") //nolint:errcheck
	ca.IssueInfraCert("beta")       //nolint:errcheck
	ca.IssueInfraCert("gamma")      //nolint:errcheck
	records := ca.GetAll()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
}

// TestRevoke_MarksRevokedAndExcludesFromGetAll verifies Revoke marks the cert
// and GetAll excludes revoked records.
func TestRevoke_MarksRevokedAndExcludesFromGetAll(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueOnboardingCert("to-revoke") //nolint:errcheck
	ca.IssueInfraCert("keep-me")        //nolint:errcheck

	if err := ca.Revoke("to-revoke"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	records := ca.GetAll()
	if len(records) != 1 {
		t.Fatalf("expected 1 non-revoked record, got %d", len(records))
	}
	if records[0].CN != "keep-me" {
		t.Errorf("expected only keep-me, got %s", records[0].CN)
	}
}

// TestRevoke_UnknownCN verifies Revoke returns an error for unknown CN.
func TestRevoke_UnknownCN(t *testing.T) {
	ca := newTestCA(t)
	if err := ca.Revoke("does-not-exist"); err == nil {
		t.Error("expected error revoking unknown CN")
	}
}

// TestRevoke_AlreadyRevoked verifies Revoke returns an error if already revoked.
func TestRevoke_AlreadyRevoked(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueOnboardingCert("double-revoke") //nolint:errcheck
	ca.Revoke("double-revoke")              //nolint:errcheck
	if err := ca.Revoke("double-revoke"); err == nil {
		t.Error("expected error revoking already-revoked cert")
	}
}

// TestReissue_UnrevokesAndEmitsIssuedEvent verifies that Reissue clears the
// revoked flag, emits an ISSUED event to subscribers, and makes the record
// visible in GetAll again.
func TestReissue_UnrevokesAndEmitsIssuedEvent(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueInfraCert("sp-reissue") //nolint:errcheck
	ca.Revoke("sp-reissue")         //nolint:errcheck

	ch, unsubscribe := ca.Subscribe()
	defer unsubscribe()

	if err := ca.Reissue("sp-reissue"); err != nil {
		t.Fatalf("Reissue: %v", err)
	}

	select {
	case event := <-ch:
		if event.Type != "issued" {
			t.Errorf("expected type=issued after reissue, got %s", event.Type)
		}
		if event.CN != "sp-reissue" {
			t.Errorf("expected CN=sp-reissue, got %s", event.CN)
		}
		if event.OU != "sy" {
			t.Errorf("expected OU=sy, got %s", event.OU)
		}
		if event.IssuedAt == "" {
			t.Error("IssuedAt should be set for reissued event")
		}
		if event.ExpiresAt == "" {
			t.Error("ExpiresAt should be set for reissued event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ISSUED event after Reissue")
	}

	// Cert must appear in GetAll now.
	records := ca.GetAll()
	found := false
	for _, r := range records {
		if r.CN == "sp-reissue" {
			found = true
		}
	}
	if !found {
		t.Error("reissued cert should appear in GetAll")
	}
}

// TestReissue_UnknownCN verifies Reissue returns an error for an unknown CN.
func TestReissue_UnknownCN(t *testing.T) {
	ca := newTestCA(t)
	if err := ca.Reissue("does-not-exist"); err == nil {
		t.Error("expected error reissuing unknown CN")
	}
}

// TestReissue_NotRevoked verifies Reissue returns an error when the cert is not revoked.
func TestReissue_NotRevoked(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueInfraCert("active-cert") //nolint:errcheck
	if err := ca.Reissue("active-cert"); err == nil {
		t.Error("expected error reissuing non-revoked cert")
	}
}

// TestReissue_RevokeReissueRevoke verifies the full revoke→reissue→revoke cycle.
func TestReissue_RevokeReissueRevoke(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueInfraCert("cycle-test") //nolint:errcheck

	if err := ca.Revoke("cycle-test"); err != nil {
		t.Fatalf("first Revoke: %v", err)
	}
	if err := ca.Reissue("cycle-test"); err != nil {
		t.Fatalf("Reissue: %v", err)
	}
	if err := ca.Revoke("cycle-test"); err != nil {
		t.Fatalf("second Revoke: %v", err)
	}

	records := ca.GetAll()
	for _, r := range records {
		if r.CN == "cycle-test" {
			t.Error("re-revoked cert should not appear in GetAll")
		}
	}
}

// TestSubscribe_IssuedEventDelivered verifies that after subscribing,
// issuing a cert sends an ISSUED event on the channel.
func TestSubscribe_IssuedEventDelivered(t *testing.T) {
	ca := newTestCA(t)
	ch, unsubscribe := ca.Subscribe()
	defer unsubscribe()

	ca.IssueOnboardingCert("subscriber-test") //nolint:errcheck

	select {
	case event := <-ch:
		if event.Type != "issued" {
			t.Errorf("expected type=issued, got %s", event.Type)
		}
		if event.CN != "subscriber-test" {
			t.Errorf("expected CN=subscriber-test, got %s", event.CN)
		}
		if event.OU != "on" {
			t.Errorf("expected OU=on, got %s", event.OU)
		}
		if event.IssuedAt == "" {
			t.Error("IssuedAt should be set for issued event")
		}
		if event.ExpiresAt == "" {
			t.Error("ExpiresAt should be set for issued event")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ISSUED event")
	}
}

// TestSubscribe_RevokedEventDelivered verifies that revoking a cert sends a REVOKED event.
func TestSubscribe_RevokedEventDelivered(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueOnboardingCert("revoke-me") //nolint:errcheck

	ch, unsubscribe := ca.Subscribe()
	defer unsubscribe()

	// Drain any buffered events (the issuance above happened before subscribe)
	// Then revoke
	if err := ca.Revoke("revoke-me"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// There may be a buffered ISSUED event; drain until we find REVOKED
	timeout := time.After(time.Second)
	for {
		select {
		case event := <-ch:
			if event.Type == "revoked" {
				if event.CN != "revoke-me" {
					t.Errorf("expected CN=revoke-me, got %s", event.CN)
				}
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for REVOKED event")
		}
	}
}

// TestSubscribe_Unsubscribe verifies that after unsubscribing,
// new events are not delivered to the unsubscribed channel.
func TestSubscribe_Unsubscribe(t *testing.T) {
	ca := newTestCA(t)
	ch, unsubscribe := ca.Subscribe()
	unsubscribe() // unsubscribe immediately

	ca.IssueOnboardingCert("after-unsub") //nolint:errcheck

	select {
	case event := <-ch:
		t.Errorf("unexpected event after unsubscribe: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// expected: no event delivered
	}
}

// TestSubscribe_MultipleSubscribers verifies fan-out to multiple subscribers.
func TestSubscribe_MultipleSubscribers(t *testing.T) {
	ca := newTestCA(t)
	ch1, unsub1 := ca.Subscribe()
	ch2, unsub2 := ca.Subscribe()
	defer unsub1()
	defer unsub2()

	ca.IssueInfraCert("fanout-test") //nolint:errcheck

	timeout := time.After(time.Second)
	for i, ch := range []<-chan CertEvent{ch1, ch2} {
		select {
		case event := <-ch:
			if event.CN != "fanout-test" {
				t.Errorf("subscriber %d: expected CN=fanout-test, got %s", i, event.CN)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for event on subscriber %d", i)
		}
	}
}
