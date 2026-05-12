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
	ca, err := NewProfileCA(24 * time.Hour)
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
