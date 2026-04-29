package service_test

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"arrowhead/core/internal/ca/model"
	"arrowhead/core/internal/ca/service"
)

func newCA(t *testing.T) *service.CAService {
	t.Helper()
	svc, err := service.NewCAService(24 * time.Hour)
	if err != nil {
		t.Fatalf("NewCAService: %v", err)
	}
	return svc
}

// ---- CAInfo ----

func TestCAInfoReturnsValidPEM(t *testing.T) {
	svc := newCA(t)
	info := svc.CAInfo()
	if info.CommonName == "" {
		t.Error("expected non-empty CommonName")
	}
	block, _ := pem.Decode([]byte(info.Certificate))
	if block == nil {
		t.Fatal("CAInfo.Certificate is not valid PEM")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		t.Fatalf("CAInfo.Certificate does not parse as X.509: %v", err)
	}
}

// ---- Issue ----

func TestIssueValid(t *testing.T) {
	svc := newCA(t)
	cert, err := svc.Issue(model.IssueRequest{SystemName: "robot-1"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if cert.SystemName != "robot-1" {
		t.Errorf("SystemName = %q, want robot-1", cert.SystemName)
	}
	if cert.Certificate == "" {
		t.Error("Certificate is empty")
	}
	if cert.PrivateKey == "" {
		t.Error("PrivateKey is empty")
	}
	if !cert.ExpiresAt.After(cert.IssuedAt) {
		t.Error("ExpiresAt is not after IssuedAt")
	}
}

func TestIssueMissingSystemName(t *testing.T) {
	svc := newCA(t)
	_, err := svc.Issue(model.IssueRequest{SystemName: ""})
	if err == nil {
		t.Fatal("expected error for missing systemName")
	}
	if err != service.ErrMissingSystemName {
		t.Errorf("expected ErrMissingSystemName, got %v", err)
	}
}

func TestIssueCustomValidDays(t *testing.T) {
	svc := newCA(t)
	cert, err := svc.Issue(model.IssueRequest{SystemName: "sensor", ValidDays: 7})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// Should expire roughly 7 days from now (within a minute of tolerance).
	want := time.Now().Add(7 * 24 * time.Hour)
	diff := cert.ExpiresAt.Sub(want)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("ExpiresAt is %v, want ~%v", cert.ExpiresAt, want)
	}
}

func TestIssueReturnsParsableCertAndKey(t *testing.T) {
	svc := newCA(t)
	issued, err := svc.Issue(model.IssueRequest{SystemName: "edge-1"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	certBlock, _ := pem.Decode([]byte(issued.Certificate))
	if certBlock == nil {
		t.Fatal("Certificate is not valid PEM")
	}
	if _, err := x509.ParseCertificate(certBlock.Bytes); err != nil {
		t.Fatalf("Certificate does not parse: %v", err)
	}

	keyBlock, _ := pem.Decode([]byte(issued.PrivateKey))
	if keyBlock == nil {
		t.Fatal("PrivateKey is not valid PEM")
	}
	if _, err := x509.ParseECPrivateKey(keyBlock.Bytes); err != nil {
		t.Fatalf("PrivateKey does not parse: %v", err)
	}
}

func TestIssueUniqueSerials(t *testing.T) {
	svc := newCA(t)
	a, _ := svc.Issue(model.IssueRequest{SystemName: "sys-a"})
	b, _ := svc.Issue(model.IssueRequest{SystemName: "sys-b"})

	parseCert := func(pemStr string) *x509.Certificate {
		block, _ := pem.Decode([]byte(pemStr))
		cert, _ := x509.ParseCertificate(block.Bytes)
		return cert
	}
	certA := parseCert(a.Certificate)
	certB := parseCert(b.Certificate)
	if certA.SerialNumber.Cmp(certB.SerialNumber) == 0 {
		t.Error("expected unique serial numbers for different certificates")
	}
}

// ---- VerifyCert ----

func TestVerifyCertValid(t *testing.T) {
	svc := newCA(t)
	issued, _ := svc.Issue(model.IssueRequest{SystemName: "trusted-system"})
	name, valid, reason := svc.VerifyCert(issued.Certificate)
	if !valid {
		t.Errorf("expected valid=true, got reason=%q", reason)
	}
	if name != "trusted-system" {
		t.Errorf("systemName = %q, want trusted-system", name)
	}
}

func TestVerifyCertInvalidPEM(t *testing.T) {
	svc := newCA(t)
	_, valid, _ := svc.VerifyCert("not-a-cert")
	if valid {
		t.Error("expected valid=false for garbage input")
	}
}

func TestVerifyCertWrongCA(t *testing.T) {
	svc1 := newCA(t)
	svc2 := newCA(t) // different CA

	issued, _ := svc1.Issue(model.IssueRequest{SystemName: "other"})
	_, valid, _ := svc2.VerifyCert(issued.Certificate)
	if valid {
		t.Error("expected valid=false for cert from different CA")
	}
}

func TestVerifyCertExpired(t *testing.T) {
	// Negative duration → cert already expired.
	svc, err := service.NewCAService(-time.Second)
	if err != nil {
		t.Fatalf("NewCAService: %v", err)
	}
	issued, err := svc.Issue(model.IssueRequest{SystemName: "expired-sys"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	_, valid, _ := svc.VerifyCert(issued.Certificate)
	if valid {
		t.Error("expected valid=false for expired cert")
	}
}
