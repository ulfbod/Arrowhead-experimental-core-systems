// tlsconfig.go — TLS configuration helpers for pki-rest-authz.
package main

import (
	"crypto/tls"
	"crypto/x509"
)

// buildServerTLSConfig creates a TLS config for the mTLS server.
func buildServerTLSConfig(cert tls.Certificate, caPool *x509.CertPool) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}
}

// buildClientTLSConfig creates a TLS config for outgoing HTTPS clients.
func buildClientTLSConfig(cert tls.Certificate, caPool *x509.CertPool) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
}
