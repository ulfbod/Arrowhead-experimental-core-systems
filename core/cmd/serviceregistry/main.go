// Arrowhead Core – Service Registry entry point.
//
// DO NOT MODIFY FOR EXPERIMENTS.
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

	"arrowhead/core/internal/api"
	"arrowhead/core/internal/config"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	cfg := config.Load()

	repo := repository.NewMemoryRepository()
	svc := service.NewRegistryService(repo)
	apiHandler := api.NewHandler(svc)

	mux := http.NewServeMux()
	mux.Handle("/serviceregistry/", apiHandler)
	mux.Handle("/health", apiHandler)

	const distDir = "dashboard/dist"
	if info, err := os.Stat(distDir); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(distDir)))
		log.Printf("[ServiceRegistry] Dashboard available at http://localhost:%s/", cfg.Port)
	}

	// Optional TLS listener on TLS_PORT.
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsCfg, err := tlsutil.LoadServerTLSConfig(
			os.Getenv("TLS_CERT_FILE"),
			os.Getenv("TLS_KEY_FILE"),
			os.Getenv("TLS_CA_FILE"),
		)
		if err != nil {
			log.Fatalf("[ServiceRegistry] TLS config: %v", err)
		}
		if tlsCfg != nil {
			go startTLS(mux, tlsPort, tlsCfg, "ServiceRegistry")
		}
	}

	log.Printf("[ServiceRegistry] Listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
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
