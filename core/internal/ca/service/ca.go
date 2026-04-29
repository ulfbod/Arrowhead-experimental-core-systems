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
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"arrowhead/core/internal/ca/model"
)

var ErrMissingSystemName = errors.New("systemName is required")

// CAService manages a self-signed CA and issues leaf certificates.
type CAService struct {
	caKey      *ecdsa.PrivateKey
	caCert     *x509.Certificate
	caCertPEM  []byte
	certDur    time.Duration
	mu         sync.Mutex
	nextSerial atomic.Int64
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
		caKey:     key,
		caCert:    cert,
		caCertPEM: certPEM,
		certDur:   certDuration,
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
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: req.SystemName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(dur),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
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
// by this CA and has not expired.
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
	return cert.Subject.CommonName, true, ""
}
