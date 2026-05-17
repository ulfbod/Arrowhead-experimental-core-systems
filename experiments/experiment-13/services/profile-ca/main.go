// profile-ca — Arrowhead 5.2 Local Cloud CA with profile hierarchy enforcement.
//
// Certificate hierarchy:
//
//	Local Cloud CA (OU=lo) — trust anchor
//	  └─ Onboarding (OU=on) — bootstrap identity
//	        └─ Device (OU=de) — device identity
//	              └─ System (OU=sy) — service identity for mTLS
//
// Three listeners:
//
//	Plain HTTP on PORT: /ca/info, /bootstrap/onboarding-cert, /ca/certificate/issue,
//	                    DELETE /ca/certificates/{cn}
//	mTLS HTTPS on TLS_PORT: /ca/device-cert, /ca/system-cert
//	gRPC on GRPC_PORT: CertificateLifecycle.Subscribe (with reflection)
//
// Environment variables:
//
//	PORT      Plain HTTP port (default: 8087)
//	TLS_PORT  mTLS HTTPS port (default: 8088)
//	GRPC_PORT gRPC port (default: 8089)
package main

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	pb "arrowhead/core-evol/proto/certlifecycle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port     := envOr("PORT", "8087")
	tlsPort  := envOr("TLS_PORT", "8088")
	grpcPort := envOr("GRPC_PORT", "8089")
	keyFile  := envOr("CA_KEY_FILE", "/data/ca.key")

	ca, err := NewProfileCA(365*24*time.Hour, keyFile)
	if err != nil {
		log.Fatalf("[profile-ca] create CA: %v", err)
	}
	log.Printf("[profile-ca] Local Cloud CA initialized (OU=lo)")

	// Plain HTTP mux: bootstrap, infra, and revocation endpoints.
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "profile-ca"})
	})
	httpMux.HandleFunc("/ca/info", handleInfo(ca))
	httpMux.HandleFunc("/bootstrap/onboarding-cert", handleBootstrapOnboarding(ca))
	httpMux.HandleFunc("/ca/certificate/issue", handleIssueInfra(ca))
	httpMux.HandleFunc("/ca/certificates/{cn}/reissue", handleReissue(ca))
	httpMux.HandleFunc("/ca/certificates/{cn}", handleRevoke(ca))

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

	// Start gRPC server with reflection.
	go func() {
		grpcLn, listenErr := net.Listen("tcp", ":"+grpcPort)
		if listenErr != nil {
			log.Fatalf("[profile-ca] listen gRPC :%s: %v", grpcPort, listenErr)
		}
		grpcSrv := grpc.NewServer()
		pb.RegisterCertificateLifecycleServer(grpcSrv, &certLifecycleServer{ca: ca})
		reflection.Register(grpcSrv)
		log.Printf("[profile-ca] gRPC listening on :%s", grpcPort)
		if serveErr := grpcSrv.Serve(grpcLn); serveErr != nil {
			log.Fatal(serveErr)
		}
	}()

	// Start mTLS HTTPS listener (main goroutine).
	tlsLn, err := tls.Listen("tcp", ":"+tlsPort, tlsCfg)
	if err != nil {
		log.Fatalf("[profile-ca] listen mTLS HTTPS :%s: %v", tlsPort, err)
	}
	log.Printf("[profile-ca] mTLS HTTPS listening on :%s", tlsPort)
	if err := http.Serve(tlsLn, tlsMux); err != nil {
		log.Fatal(err)
	}
}
