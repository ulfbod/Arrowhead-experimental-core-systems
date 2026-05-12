// profile-ca — Arrowhead 5.2 Local Cloud CA with profile hierarchy enforcement.
//
// Certificate hierarchy:
//   Local Cloud CA (OU=lo) — trust anchor
//     └─ Onboarding (OU=on) — bootstrap identity
//           └─ Device (OU=de) — device identity
//                 └─ System (OU=sy) — service identity for mTLS
//
// Two listeners:
//   Plain HTTP on PORT: /ca/info, /bootstrap/onboarding-cert, /ca/certificate/issue
//   mTLS HTTPS on TLS_PORT: /ca/device-cert, /ca/system-cert
//
// Environment variables:
//   PORT      Plain HTTP port (default: 8087)
//   TLS_PORT  mTLS HTTPS port (default: 8088)
package main

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port    := envOr("PORT", "8087")
	tlsPort := envOr("TLS_PORT", "8088")

	ca, err := NewProfileCA(365 * 24 * time.Hour)
	if err != nil {
		log.Fatalf("[profile-ca] create CA: %v", err)
	}
	log.Printf("[profile-ca] Local Cloud CA initialized (OU=lo)")

	// Plain HTTP mux: bootstrap and infra endpoints.
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "profile-ca"})
	})
	httpMux.HandleFunc("/ca/info", handleInfo(ca))
	httpMux.HandleFunc("/bootstrap/onboarding-cert", handleBootstrapOnboarding(ca))
	httpMux.HandleFunc("/ca/certificate/issue", handleIssueInfra(ca))

	// mTLS HTTPS mux: profile-enforced endpoints.
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/ca/device-cert", handleDeviceCert(ca))
	tlsMux.HandleFunc("/ca/system-cert", handleSystemCert(ca))

	// Build TLS config: CA's own cert+key as server cert; require client cert.
	tlsCert, err := ca.TLSCert()
	if err != nil {
		log.Fatalf("[profile-ca] build TLS cert: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(ca.CACert())
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}

	// Start plain HTTP listener.
	go func() {
		ln, listenErr := net.Listen("tcp", ":"+port)
		if listenErr != nil {
			log.Fatalf("[profile-ca] listen HTTP :%s: %v", port, listenErr)
		}
		log.Printf("[profile-ca] HTTP listening on :%s", port)
		if serveErr := http.Serve(ln, httpMux); serveErr != nil {
			log.Fatal(serveErr)
		}
	}()

	// Start mTLS HTTPS listener.
	tlsLn, err := tls.Listen("tcp", ":"+tlsPort, tlsCfg)
	if err != nil {
		log.Fatalf("[profile-ca] listen mTLS HTTPS :%s: %v", tlsPort, err)
	}
	log.Printf("[profile-ca] mTLS HTTPS listening on :%s", tlsPort)
	if err := http.Serve(tlsLn, tlsMux); err != nil {
		log.Fatal(err)
	}
}
