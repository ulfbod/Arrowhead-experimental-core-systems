// Package service implements the Certificate Authority business logic.
//
// The CA generates a self-signed ECDSA P-256 root certificate at startup and
// uses it to sign leaf certificates for systems that request onboarding.
// All state is in-memory (see GAP_ANALYSIS.md G5).
package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"arrowhead/core/internal/ca/model"
)

var (
	ErrMissingSystemName  = errors.New("systemName is required")
	ErrMissingCertificate = errors.New("certificate is required")
	ErrCertNotIssuedByCA  = errors.New("certificate was not issued by this CA")
)

// revokedEntry records when a certificate serial was revoked.
type revokedEntry struct {
	serial     *big.Int
	revokedAt  time.Time
	systemName string
}

// CAService manages a self-signed CA and issues leaf certificates.
type CAService struct {
	caKey      *ecdsa.PrivateKey
	caCert     *x509.Certificate
	caCertPEM  []byte
	certDur    time.Duration
	mu         sync.Mutex
	nextSerial atomic.Int64

	// revocation: protected by revokedMu
	revokedMu   sync.RWMutex
	revokedList []revokedEntry      // ordered by revocation time
	revokedSet  map[string]struct{} // serial.String() → revoked; fast membership test
}

// NewCAService initialises the CA by generating a self-signed root certificate.
// certDuration is the default lifetime of issued leaf certificates;
// pass a negative value in tests to produce immediately-expired certificates.
func NewCAService(certDuration time.Duration) (*CAService, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Arrowhead Local Cloud CA",
			Organization: []string{"Arrowhead"},
		},
		NotBefore:             time.Now().Add(-time.Minute), // small clock-skew tolerance
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	svc := &CAService{
		caKey:      key,
		caCert:     cert,
		caCertPEM:  certPEM,
		certDur:    certDuration,
		revokedSet: make(map[string]struct{}),
	}
	svc.nextSerial.Store(2) // 1 is the CA itself
	return svc, nil
}

// Issue generates a new leaf certificate for the given system and returns the
// PEM-encoded certificate and private key.
func (s *CAService) Issue(req model.IssueRequest) (*model.IssuedCert, error) {
	if req.SystemName == "" {
		return nil, ErrMissingSystemName
	}

	dur := s.certDur
	if req.ValidDays > 0 {
		dur = time.Duration(req.ValidDays) * 24 * time.Hour
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial := s.nextSerial.Add(1)
	now := time.Now()

	// Build the Subject CN and DNS SANs.
	// When cloudName and operatorName are provided, form the AH5 hierarchical name:
	//   systemName.cloudName.operatorName.arrowhead.eu
	// Both the bare system name and the hierarchical name are included as DNS SANs so
	// that TLS hostname verification works for both Docker hostnames and AH5-compliant names.
	cn := req.SystemName
	dnsNames := []string{req.SystemName}
	if req.CloudName != "" && req.OperatorName != "" {
		hierarchical := fmt.Sprintf("%s.%s.%s.arrowhead.eu", req.SystemName, req.CloudName, req.OperatorName)
		cn = hierarchical
		dnsNames = append(dnsNames, hierarchical)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		// Go 1.15+ requires SANs for hostname verification; CN alone is rejected.
		// Always include the bare system name as a DNS SAN for Docker hostname verification.
		// When hierarchical naming is used, the AH5 name is also included.
		DNSNames:    dnsNames,
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(dur),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}

	s.mu.Lock()
	der, err := x509.CreateCertificate(rand.Reader, template, s.caCert, &leafKey.PublicKey, s.caKey)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyBytes, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return &model.IssuedCert{
		SystemName:  req.SystemName,
		Certificate: string(certPEM),
		PrivateKey:  string(keyPEM),
		IssuedAt:    now,
		ExpiresAt:   now.Add(dur),
	}, nil
}

// CAInfo returns the CA's own certificate in PEM form.
func (s *CAService) CAInfo() model.CAInfo {
	return model.CAInfo{
		CommonName:  s.caCert.Subject.CommonName,
		Certificate: string(s.caCertPEM),
	}
}

// VerifyCert parses a PEM-encoded leaf certificate and checks it was signed
// by this CA, has not expired, and has not been revoked.
func (s *CAService) VerifyCert(certPEM string) (systemName string, valid bool, reason string) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", false, "invalid PEM"
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", false, "cannot parse certificate"
	}
	pool := x509.NewCertPool()
	pool.AddCert(s.caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		return "", false, err.Error()
	}

	// Check revocation after chain verification.
	s.revokedMu.RLock()
	_, revoked := s.revokedSet[cert.SerialNumber.String()]
	s.revokedMu.RUnlock()
	if revoked {
		return cert.Subject.CommonName, false, "certificate has been revoked"
	}

	return cert.Subject.CommonName, true, ""
}

// Revoke records a certificate as revoked. The certificate must have been issued
// by this CA. Revoking an already-revoked certificate is a no-op (idempotent).
func (s *CAService) Revoke(certPEM string) (*model.RevokeResponse, error) {
	if certPEM == "" {
		return nil, ErrMissingCertificate
	}
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, errors.New("invalid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cannot parse certificate: %w", err)
	}

	// Verify the cert belongs to this CA before accepting revocation.
	pool := x509.NewCertPool()
	pool.AddCert(s.caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCertNotIssuedByCA, err.Error())
	}

	now := time.Now()
	key := cert.SerialNumber.String()

	s.revokedMu.Lock()
	if _, already := s.revokedSet[key]; !already {
		s.revokedSet[key] = struct{}{}
		s.revokedList = append(s.revokedList, revokedEntry{
			serial:     cert.SerialNumber,
			revokedAt:  now,
			systemName: cert.Subject.CommonName,
		})
	}
	s.revokedMu.Unlock()

	return &model.RevokeResponse{
		SystemName: cert.Subject.CommonName,
		RevokedAt:  now.UTC().Format(time.RFC3339),
	}, nil
}

// CRL generates and returns a PEM-encoded Certificate Revocation List signed by
// this CA. The CRL is generated fresh on each call; it is valid for 24 hours.
func (s *CAService) CRL() ([]byte, error) {
	s.revokedMu.RLock()
	entries := make([]x509.RevocationListEntry, len(s.revokedList))
	for i, e := range s.revokedList {
		entries[i] = x509.RevocationListEntry{
			SerialNumber:   e.serial,
			RevocationTime: e.revokedAt,
		}
	}
	s.revokedMu.RUnlock()

	template := &x509.RevocationList{
		Number:                    big.NewInt(time.Now().Unix()),
		ThisUpdate:                time.Now(),
		NextUpdate:                time.Now().Add(24 * time.Hour),
		RevokedCertificateEntries: entries,
	}

	s.mu.Lock()
	der, err := x509.CreateRevocationList(rand.Reader, template, s.caCert, s.caKey)
	s.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("create CRL: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}), nil
}
