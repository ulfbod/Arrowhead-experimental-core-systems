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

// ---- Hierarchical naming ----

func TestIssueHierarchicalCN(t *testing.T) {
	svc := newCA(t)
	req := model.IssueRequest{SystemName: "sensor-1", CloudName: "testcloud", OperatorName: "testorg"}
	issued, err := svc.Issue(req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	block, _ := pem.Decode([]byte(issued.Certificate))
	if block == nil {
		t.Fatal("Certificate is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	wantCN := "sensor-1.testcloud.testorg.arrowhead.eu"
	if cert.Subject.CommonName != wantCN {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, wantCN)
	}

	// Both bare name and hierarchical name must appear as DNS SANs.
	sanSet := make(map[string]bool)
	for _, s := range cert.DNSNames {
		sanSet[s] = true
	}
	if !sanSet["sensor-1"] {
		t.Errorf("bare system name not in DNS SANs: %v", cert.DNSNames)
	}
	if !sanSet[wantCN] {
		t.Errorf("hierarchical name %q not in DNS SANs: %v", wantCN, cert.DNSNames)
	}
}

func TestIssueBareNameWhenNoHierarchy(t *testing.T) {
	svc := newCA(t)
	issued, err := svc.Issue(model.IssueRequest{SystemName: "robot-1"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	block, _ := pem.Decode([]byte(issued.Certificate))
	cert, _ := x509.ParseCertificate(block.Bytes)

	if cert.Subject.CommonName != "robot-1" {
		t.Errorf("CN = %q, want robot-1", cert.Subject.CommonName)
	}
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "robot-1" {
		t.Errorf("DNS SANs = %v, want [robot-1]", cert.DNSNames)
	}
}

func TestIssueHierarchicalCNVerifiesOK(t *testing.T) {
	svc := newCA(t)
	req := model.IssueRequest{SystemName: "gw", CloudName: "cloud1", OperatorName: "myorg"}
	issued, _ := svc.Issue(req)

	name, valid, reason := svc.VerifyCert(issued.Certificate)
	if !valid {
		t.Errorf("expected valid=true, got reason=%q", reason)
	}
	// VerifyCert returns the CN (which is the hierarchical name).
	want := "gw.cloud1.myorg.arrowhead.eu"
	if name != want {
		t.Errorf("systemName = %q, want %q", name, want)
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

// ---- Revoke ----

func TestRevokeValidCert(t *testing.T) {
	svc := newCA(t)
	issued, _ := svc.Issue(model.IssueRequest{SystemName: "to-revoke"})

	// Before revocation: valid.
	_, valid, _ := svc.VerifyCert(issued.Certificate)
	if !valid {
		t.Fatal("expected cert to be valid before revocation")
	}

	resp, err := svc.Revoke(issued.Certificate)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if resp.SystemName != "to-revoke" {
		t.Errorf("SystemName = %q, want to-revoke", resp.SystemName)
	}
	if resp.RevokedAt == "" {
		t.Error("RevokedAt is empty")
	}

	// After revocation: invalid.
	_, valid, reason := svc.VerifyCert(issued.Certificate)
	if valid {
		t.Error("expected cert to be invalid after revocation")
	}
	if reason != "certificate has been revoked" {
		t.Errorf("reason = %q, want 'certificate has been revoked'", reason)
	}
}

func TestRevokeIdempotent(t *testing.T) {
	svc := newCA(t)
	issued, _ := svc.Issue(model.IssueRequest{SystemName: "idempotent"})

	// Revoke twice: should not error.
	if _, err := svc.Revoke(issued.Certificate); err != nil {
		t.Fatalf("first Revoke: %v", err)
	}
	if _, err := svc.Revoke(issued.Certificate); err != nil {
		t.Fatalf("second Revoke: %v", err)
	}
}

func TestRevokeEmptyCert(t *testing.T) {
	svc := newCA(t)
	_, err := svc.Revoke("")
	if err == nil {
		t.Error("expected error for empty certificate")
	}
}

func TestRevokeForeignCert(t *testing.T) {
	svc1 := newCA(t)
	svc2 := newCA(t)
	issued, _ := svc1.Issue(model.IssueRequest{SystemName: "foreign"})
	_, err := svc2.Revoke(issued.Certificate)
	if err == nil {
		t.Error("expected error when revoking cert from different CA")
	}
}

// ---- CRL ----

func TestCRLEmptyList(t *testing.T) {
	svc := newCA(t)
	crlPEM, err := svc.CRL()
	if err != nil {
		t.Fatalf("CRL: %v", err)
	}
	if len(crlPEM) == 0 {
		t.Error("CRL PEM is empty")
	}
	block, _ := pem.Decode(crlPEM)
	if block == nil {
		t.Fatal("CRL is not valid PEM")
	}
	if block.Type != "X509 CRL" {
		t.Errorf("PEM block type = %q, want X509 CRL", block.Type)
	}
	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		t.Fatalf("ParseRevocationList: %v", err)
	}
	if len(crl.RevokedCertificateEntries) != 0 {
		t.Errorf("expected 0 revoked entries, got %d", len(crl.RevokedCertificateEntries))
	}
}

func TestCRLContainsRevokedSerial(t *testing.T) {
	svc := newCA(t)
	issued, _ := svc.Issue(model.IssueRequest{SystemName: "revoked-sys"})
	svc.Revoke(issued.Certificate) //nolint:errcheck

	crlPEM, err := svc.CRL()
	if err != nil {
		t.Fatalf("CRL: %v", err)
	}
	block, _ := pem.Decode(crlPEM)
	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		t.Fatalf("ParseRevocationList: %v", err)
	}
	if len(crl.RevokedCertificateEntries) != 1 {
		t.Fatalf("expected 1 revoked entry, got %d", len(crl.RevokedCertificateEntries))
	}

	// Parse the issued cert to get its serial and compare.
	certBlock, _ := pem.Decode([]byte(issued.Certificate))
	cert, _ := x509.ParseCertificate(certBlock.Bytes)
	if crl.RevokedCertificateEntries[0].SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Errorf("CRL serial = %v, want %v",
			crl.RevokedCertificateEntries[0].SerialNumber, cert.SerialNumber)
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
