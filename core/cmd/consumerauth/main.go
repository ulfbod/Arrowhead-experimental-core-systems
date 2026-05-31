// Arrowhead Core – ConsumerAuthorization System entry point.
// Default port: 8082 (set PORT env var to override).
//
// Optional mutual TLS:
//
//	TLS_PORT      — HTTPS listen port (when set, also starts an HTTPS listener)
//	TLS_CERT_FILE — PEM certificate file (required when TLS_PORT is set)
//	TLS_KEY_FILE  — PEM private key file (required when TLS_PORT is set)
//	TLS_CA_FILE   — PEM CA certificate file; when set, mutual TLS is enforced
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"arrowhead/core/internal/consumerauth/api"
	"arrowhead/core/internal/consumerauth/repository"
	"arrowhead/core/internal/consumerauth/service"
	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/generalmgmt"
	"arrowhead/core/internal/tlsutil"
)


func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var repo repository.Repository
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		var err error
		sqliteRepo, err := repository.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[ConsumerAuthorization] open database: %v", err)
		}
		repo = sqliteRepo
	} else {
		repo = repository.NewMemoryRepository()
	}
	svc := service.NewAuthService(repo)

	// Optional RSA key pair for JWT signing (G47).
	// When JWT_PRIVATE_KEY_FILE is set, load and use the PEM-encoded RSA private key.
	// When unset, an ephemeral key (generated at startup) is used — tokens are valid only
	// for the current process lifetime.
	if keyFile := os.Getenv("JWT_PRIVATE_KEY_FILE"); keyFile != "" {
		keyPEM, err := os.ReadFile(keyFile)
		if err != nil {
			log.Fatalf("[ConsumerAuthorization] read JWT_PRIVATE_KEY_FILE: %v", err)
		}
		rsaKey, err := service.ParseRSAPrivateKey(keyPEM)
		if err != nil {
			log.Fatalf("[ConsumerAuthorization] parse RSA private key: %v", err)
		}
		svc.SetRSAKey(rsaKey)
	}

	var blClient blclient.BlacklistClient = blclient.NopClient{}
	if blURL := os.Getenv("BLACKLIST_URL"); blURL != "" {
		blClient = blclient.NewHTTPClient(blURL, http.DefaultClient)
	}
	sysHandler := api.NewHandler(svc, os.Getenv("MGMT_AUTH_URL"), blClient, os.Getenv("TOKEN_AUTH_URL"))

	mgmtHandler := generalmgmt.NewHandler(buf, "authorization", map[string]string{
		"PORT":           port,
		"DB_PATH":        os.Getenv("DB_PATH"),
		"TLS_PORT":       os.Getenv("TLS_PORT"),
		"MGMT_AUTH_URL":  os.Getenv("MGMT_AUTH_URL"),
		"BLACKLIST_URL":  os.Getenv("BLACKLIST_URL"),
		"TOKEN_AUTH_URL": os.Getenv("TOKEN_AUTH_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/authorization/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	tlsCfg, err := tlsutil.LoadServerTLSConfig(
		os.Getenv("TLS_CERT_FILE"),
		os.Getenv("TLS_KEY_FILE"),
		os.Getenv("TLS_CA_FILE"),
	)
	if err != nil {
		log.Fatalf("[ConsumerAuthorization] TLS config: %v", err)
	}
	httpsOnly := os.Getenv("HTTPS_ONLY") == "true"
	tlsAddr := ""
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsAddr = ":" + tlsPort
	}

	slog.Info("Listening", "system", "ConsumerAuthorization", "port", port)
	log.Fatal(tlsutil.ServeHTTPS(":"+port, tlsAddr, root, tlsCfg, httpsOnly))
}
