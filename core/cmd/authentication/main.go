// Arrowhead Core – Authentication System entry point.
// Default port: 8081 (set PORT env var to override).
//
// Optional mutual TLS:
//
//	TLS_PORT      — HTTPS listen port (when set, also starts an HTTPS listener)
//	TLS_CERT_FILE — PEM certificate file (required when TLS_PORT is set)
//	TLS_KEY_FILE  — PEM private key file (required when TLS_PORT is set)
//	TLS_CA_FILE   — PEM CA certificate file; when set, mutual TLS is enforced
package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	"arrowhead/core/internal/authentication/api"
	"arrowhead/core/internal/authentication/repository"
	"arrowhead/core/internal/authentication/service"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	tokenDuration := 3600 * time.Second // 1 hour default

	repo := repository.NewMemoryRepository()
	svc := service.NewAuthService(repo, tokenDuration)
	handler := api.NewHandler(svc)

	// Optional TLS listener on TLS_PORT.
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsCfg, err := tlsutil.LoadServerTLSConfig(
			os.Getenv("TLS_CERT_FILE"),
			os.Getenv("TLS_KEY_FILE"),
			os.Getenv("TLS_CA_FILE"),
		)
		if err != nil {
			log.Fatalf("[Authentication] TLS config: %v", err)
		}
		if tlsCfg != nil {
			go startTLS(handler, tlsPort, tlsCfg, "Authentication")
		}
	}

	log.Printf("[Authentication] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func startTLS(handler http.Handler, port string, tlsCfg *tls.Config, name string) {
	ln, err := tls.Listen("tcp", ":"+port, tlsCfg)
	if err != nil {
		log.Fatalf("[%s] TLS listen on :%s: %v", name, port, err)
	}
	log.Printf("[%s] Listening on :%s (HTTPS/mTLS)", name, port)
	if err := http.Serve(ln, handler); err != nil {
		log.Fatalf("[%s] TLS serve: %v", name, err)
	}
}
