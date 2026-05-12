// ca.go — Arrowhead 5.2 profile-based Local Cloud CA.
//
// Certificate profiles encoded in Subject OrganizationalUnit (OU):
//   lo — Local Cloud CA (root)
//   on — Onboarding  (may request Device certs)
//   de — Device      (may request System certs)
//   sy — System      (used for service-to-service mTLS)
//
// Issuance rules strictly enforced:
//   HTTP bootstrap → on
//   on client cert → de
//   de client cert → sy
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

// CertProfile is an Arrowhead 5.2 certificate profile tier.
type CertProfile string

const (
	ProfileOnboarding CertProfile = "on"
	ProfileDevice     CertProfile = "de"
	ProfileSystem     CertProfile = "sy"
)

// ProfileCA is the Local Cloud Certificate Authority with profile enforcement.
type ProfileCA struct {
	caKey      *ecdsa.PrivateKey
	caCert     *x509.Certificate
	caCertPEM  []byte
	certDur    time.Duration
	mu         sync.Mutex
	nextSerial atomic.Int64
}

// NewProfileCA creates a new Local Cloud CA with the given cert lifetime.
func NewProfileCA(certDuration time.Duration) (*ProfileCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "Arrowhead Local Cloud CA",
			Organization:       []string{"Arrowhead"},
			OrganizationalUnit: []string{"lo"},
		},
		// SANs needed so Go TLS accepts the CA cert as a server cert on the mTLS port.
		DNSNames:              []string{"profile-ca", "localhost"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	ca := &ProfileCA{
		caKey:     key,
		caCert:    cert,
		caCertPEM: certPEM,
		certDur:   certDuration,
	}
	ca.nextSerial.Store(2)
	return ca, nil
}

// issueCert is the internal cert issuance. Profile is set in OU.
func (ca *ProfileCA) issueCert(systemName string, profile CertProfile) (certPEM, keyPEM string, err error) {
	if systemName == "" {
		return "", "", errors.New("systemName is required")
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}

	serial := ca.nextSerial.Add(1)
	now := time.Now()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:         systemName,
			Organization:       []string{"Arrowhead"},
			OrganizationalUnit: []string{string(profile)},
		},
		DNSNames:    []string{systemName},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(ca.certDur),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}

	ca.mu.Lock()
	der, err := x509.CreateCertificate(rand.Reader, template, ca.caCert, &leafKey.PublicKey, ca.caKey)
	ca.mu.Unlock()
	if err != nil {
		return "", "", fmt.Errorf("create certificate: %w", err)
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyBytes, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal key: %w", err)
	}
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return string(certPEMBytes), string(keyPEMBytes), nil
}

// IssueOnboardingCert issues an Onboarding certificate (OU=on).
// Available over plain HTTP — no authentication required.
func (ca *ProfileCA) IssueOnboardingCert(systemName string) (certPEM, keyPEM string, err error) {
	return ca.issueCert(systemName, ProfileOnboarding)
}

// IssueDeviceCert issues a Device certificate (OU=de).
// Requires the requester to present a valid Onboarding certificate (OU=on).
func (ca *ProfileCA) IssueDeviceCert(systemName string, requesterCert *x509.Certificate) (certPEM, keyPEM string, err error) {
	if err := ca.verifyProfile(requesterCert, ProfileOnboarding); err != nil {
		return "", "", fmt.Errorf("requester profile: %w", err)
	}
	return ca.issueCert(systemName, ProfileDevice)
}

// IssueSystemCert issues a System certificate (OU=sy).
// Requires the requester to present a valid Device certificate (OU=de).
func (ca *ProfileCA) IssueSystemCert(systemName string, requesterCert *x509.Certificate) (certPEM, keyPEM string, err error) {
	if err := ca.verifyProfile(requesterCert, ProfileDevice); err != nil {
		return "", "", fmt.Errorf("requester profile: %w", err)
	}
	return ca.issueCert(systemName, ProfileSystem)
}

// IssueInfraCert issues a System certificate without profile chain enforcement.
// This backward-compatible endpoint is used by cert-provisioner for Kafka,
// RabbitMQ, and core system certificate files.
func (ca *ProfileCA) IssueInfraCert(systemName string) (certPEM, keyPEM string, err error) {
	return ca.issueCert(systemName, ProfileSystem)
}

// verifyProfile checks that cert was issued by this CA and has the expected OU profile.
func (ca *ProfileCA) verifyProfile(cert *x509.Certificate, expected CertProfile) error {
	pool := x509.NewCertPool()
	pool.AddCert(ca.caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		return fmt.Errorf("chain invalid: %w", err)
	}
	for _, ou := range cert.Subject.OrganizationalUnit {
		if CertProfile(ou) == expected {
			return nil
		}
	}
	return fmt.Errorf("expected profile %q, got %v", expected, cert.Subject.OrganizationalUnit)
}

// CACertPEM returns the CA certificate in PEM format.
func (ca *ProfileCA) CACertPEM() string { return string(ca.caCertPEM) }

// CACert returns the parsed CA certificate.
func (ca *ProfileCA) CACert() *x509.Certificate { return ca.caCert }

// TLSCert returns a tls.Certificate using the CA's own cert+key for serving mTLS.
func (ca *ProfileCA) TLSCert() (tls.Certificate, error) {
	keyBytes, err := x509.MarshalECPrivateKey(ca.caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal CA key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return tls.X509KeyPair(ca.caCertPEM, keyPEM)
}
