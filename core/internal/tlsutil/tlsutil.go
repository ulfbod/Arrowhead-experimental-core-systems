// Package tlsutil provides helpers for loading TLS configuration from PEM files.
//
// Core service binaries use these helpers to support optional mutual TLS:
// when TLS_CERT_FILE, TLS_KEY_FILE, and optionally TLS_CA_FILE are set in the
// environment, the service starts an HTTPS listener in addition to (or instead
// of) the plain HTTP listener.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// LoadServerTLSConfig returns a *tls.Config for an HTTPS server.
//
// certFile and keyFile must be non-empty paths to PEM-encoded certificate and
// private key files. If caFile is non-empty, ClientAuth is set to
// tls.RequireAndVerifyClientCert so the server demands a valid client
// certificate on every connection (mutual TLS).
//
// Returns (nil, nil) when certFile and keyFile are both empty (TLS disabled).
// Returns an error if any file cannot be read or parsed.
func LoadServerTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load server key pair (%s, %s): %w", certFile, keyFile, err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if caFile != "" {
		pool, err := loadCertPool(caFile)
		if err != nil {
			return nil, fmt.Errorf("load CA cert pool: %w", err)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}

// LoadClientTLSConfig returns a *tls.Config for an HTTPS client.
//
// caFile is required for server certificate verification; the function returns
// (nil, nil) when caFile is empty (TLS disabled).
//
// certFile and keyFile are optional: when both are non-empty, the client
// presents a certificate during the TLS handshake (mutual TLS).
func LoadClientTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if caFile == "" {
		return nil, nil
	}
	pool, err := loadCertPool(caFile)
	if err != nil {
		return nil, fmt.Errorf("load CA cert pool: %w", err)
	}
	cfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load client key pair (%s, %s): %w", certFile, keyFile, err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

// NewHTTPClient returns an *http.Client configured with the given TLS config.
//
// When tlsCfg is nil (TLS disabled), returns a plain http.Client with a
// 5-second timeout and no custom transport.
// When tlsCfg is non-nil, wraps it in an http.Transport.
func NewHTTPClient(tlsCfg *tls.Config) *http.Client {
	if tlsCfg == nil {
		return &http.Client{Timeout: 5 * time.Second}
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   5 * time.Second,
	}
}

// ServeHTTPS starts HTTP and/or HTTPS listeners according to the httpsOnly flag.
//
// When httpsOnly is false OR tlsCfg is nil (fail-safe):
//   - If tlsCfg is non-nil, a TLS listener is started in a goroutine on tlsAddr.
//   - The function blocks on plain HTTP at plainAddr.
//
// When httpsOnly is true AND tlsCfg is non-nil:
//   - A health-only mux is started in a goroutine on plainAddr.
//     /health → 200 {"status":"UP"}; all other paths → 451 (plain HTTP disabled).
//   - The function blocks on TLS at tlsAddr.
func ServeHTTPS(plainAddr, tlsAddr string, handler http.Handler, tlsCfg *tls.Config, httpsOnly bool) error {
	if httpsOnly && tlsCfg != nil {
		// Start health-only plain listener in goroutine.
		healthMux := http.NewServeMux()
		healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"UP"}`)) //nolint:errcheck
		})
		healthMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnavailableForLegalReasons)
			w.Write([]byte(`{"errorMessage":"plain HTTP disabled; use HTTPS port","origin":"security"}`)) //nolint:errcheck
		})
		go http.ListenAndServe(plainAddr, healthMux) //nolint:errcheck

		// Block on TLS listener.
		srv := &http.Server{
			Addr:      tlsAddr,
			Handler:   handler,
			TLSConfig: tlsCfg,
		}
		return srv.ListenAndServeTLS("", "")
	}

	// Default path: optional TLS in goroutine, block on plain HTTP.
	if tlsCfg != nil && tlsAddr != "" {
		go func() {
			ln, err := tls.Listen("tcp", tlsAddr, tlsCfg)
			if err != nil {
				return
			}
			http.Serve(ln, handler) //nolint:errcheck
		}()
	}
	return http.ListenAndServe(plainAddr, handler)
}

// loadCertPool reads a PEM file and returns an x509.CertPool containing it.
func loadCertPool(caFile string) (*x509.CertPool, error) {
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("parse CA PEM from %q: no valid certificates found", caFile)
	}
	return pool, nil
}
