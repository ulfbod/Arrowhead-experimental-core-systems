package main

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"
)

func TestBuildServerTLSConfig(t *testing.T) {
	cert := makeTLSCert(t, "server")
	pool := x509.NewCertPool()
	cfg := buildServerTLSConfig(cert, pool)
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("expected RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
	}
	if cfg.ClientCAs != pool {
		t.Error("expected CA pool set")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected TLS 1.2 minimum, got %v", cfg.MinVersion)
	}
}

func TestBuildClientTLSConfig(t *testing.T) {
	cert := makeTLSCert(t, "client")
	pool := x509.NewCertPool()
	cfg := buildClientTLSConfig(cert, pool)
	if cfg.RootCAs != pool {
		t.Error("expected RootCAs set")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected TLS 1.2 minimum, got %v", cfg.MinVersion)
	}
}

func TestBuildMTLSUpstreamClient(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	client := buildMTLSUpstreamClient(tlsCfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if _, ok := client.Transport.(*http.Transport); !ok {
		t.Error("expected *http.Transport")
	}
}
