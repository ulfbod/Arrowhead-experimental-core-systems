// ca.go — Arrowhead 5.2 profile-based Local Cloud CA.
//
// Certificate profiles encoded in Subject OrganizationalUnit (OU):
//
//	lo — Local Cloud CA (root)
//	on — Onboarding  (may request Device certs)
//	de — Device      (may request System certs)
//	sy — System      (used for service-to-service mTLS)
//
// Issuance rules strictly enforced:
//
//	HTTP bootstrap → on
//	on client cert → de
//	de client cert → sy
//
// Experiment-13 additions:
//   - CertRecord registry (in-memory, protected by mu)
//   - CertEvent fan-out via Subscribe/Unsubscribe
//   - Revoke(cn) — marks revoked, emits REVOKED event
//   - GetAll() — returns all non-revoked records (snapshot)
//   - Subscribe() — returns (channel, unsubscribeFn)
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
	"log"
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

// CertRecord holds registry metadata for an issued certificate.
type CertRecord struct {
	CN        string
	OU        string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Revoked   bool
}

// CertEvent is the internal Go type for certificate lifecycle events.
// The gRPC server (grpc.go) translates this to the proto type.
type CertEvent struct {
	CN        string
	OU        string
	Type      string // "issued", "revoked", "expired", "snapshot"
	IssuedAt  string // RFC3339, empty for revoked/expired
	ExpiresAt string // RFC3339, empty for revoked/expired
}

// ProfileCA is the Local Cloud Certificate Authority with profile enforcement.
type ProfileCA struct {
	caKey      *ecdsa.PrivateKey
	caCert     *x509.Certificate
	caCertPEM  []byte
	certDur    time.Duration
	mu         sync.Mutex
	nextSerial atomic.Int64

	// Experiment-13: cert registry and subscriber fan-out (protected by mu).
	records     map[string]*CertRecord
	subscribers []chan CertEvent
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
		records:   make(map[string]*CertRecord),
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
	der, createErr := x509.CreateCertificate(rand.Reader, template, ca.caCert, &leafKey.PublicKey, ca.caKey)
	if createErr == nil {
		// Register the cert record and fan-out the event while holding the lock.
		issuedAt := now
		expiresAt := now.Add(ca.certDur)
		rec := &CertRecord{
			CN:        systemName,
			OU:        string(profile),
			IssuedAt:  issuedAt,
			ExpiresAt: expiresAt,
			Revoked:   false,
		}
		ca.records[systemName] = rec
		event := CertEvent{
			CN:        systemName,
			OU:        string(profile),
			Type:      "issued",
			IssuedAt:  issuedAt.UTC().Format(time.RFC3339),
			ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		}
		ca.fanOut(event)
	}
	ca.mu.Unlock()

	if createErr != nil {
		return "", "", fmt.Errorf("create certificate: %w", createErr)
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyBytes, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal key: %w", err)
	}
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return string(certPEMBytes), string(keyPEMBytes), nil
}

// fanOut sends event to all subscriber channels non-blocking.
// Caller must hold ca.mu.
func (ca *ProfileCA) fanOut(event CertEvent) {
	for _, ch := range ca.subscribers {
		select {
		case ch <- event:
		default:
			log.Printf("[profile-ca] WARNING: subscriber channel full, dropping event CN=%s type=%s", event.CN, event.Type)
		}
	}
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

// Revoke marks the certificate with the given CN as revoked and emits a REVOKED event.
// Returns an error if CN is not found or is already revoked.
func (ca *ProfileCA) Revoke(cn string) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	rec, ok := ca.records[cn]
	if !ok {
		return fmt.Errorf("certificate not found: %s", cn)
	}
	if rec.Revoked {
		return fmt.Errorf("certificate already revoked: %s", cn)
	}
	rec.Revoked = true
	event := CertEvent{
		CN:   cn,
		OU:   rec.OU,
		Type: "revoked",
	}
	ca.fanOut(event)
	return nil
}

// GetAll returns all non-revoked certificate records (snapshot).
func (ca *ProfileCA) GetAll() []*CertRecord {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	out := make([]*CertRecord, 0, len(ca.records))
	for _, rec := range ca.records {
		if !rec.Revoked {
			out = append(out, rec)
		}
	}
	return out
}

// Subscribe returns a buffered channel (size 64) that receives CertEvents,
// and an unsubscribe function. Call the unsubscribe function to stop delivery.
func (ca *ProfileCA) Subscribe() (<-chan CertEvent, func()) {
	ch := make(chan CertEvent, 64)
	ca.mu.Lock()
	ca.subscribers = append(ca.subscribers, ch)
	ca.mu.Unlock()

	unsubscribe := func() {
		ca.mu.Lock()
		defer ca.mu.Unlock()
		for i, s := range ca.subscribers {
			if s == ch {
				ca.subscribers = append(ca.subscribers[:i], ca.subscribers[i+1:]...)
				break
			}
		}
	}
	return ch, unsubscribe
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
