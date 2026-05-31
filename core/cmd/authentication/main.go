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
	"log/slog"
	"net/http"
	"os"
	"time"

	"arrowhead/core/internal/authentication/api"
	"arrowhead/core/internal/authentication/repository"
	"arrowhead/core/internal/authentication/service"
	"arrowhead/core/internal/generalmgmt"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	tokenDuration := 3600 * time.Second // 1 hour default

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var tokenRepo repository.Repository
	var identityRepo repository.IdentityRepository
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		sqliteRepo, err := repository.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[Authentication] open token database: %v", err)
		}
		tokenRepo = sqliteRepo
		sqliteIdentityRepo, err := repository.NewSQLiteIdentityRepository(dbPath + ".identities")
		if err != nil {
			log.Fatalf("[Authentication] open identity database: %v", err)
		}
		identityRepo = sqliteIdentityRepo
	} else {
		tokenRepo = repository.NewMemoryRepository()
		identityRepo = repository.NewMemoryIdentityRepository()
	}
	svc := service.NewAuthServiceFull(tokenRepo, identityRepo, tokenDuration)
	sysHandler := api.NewHandler(svc, os.Getenv("MGMT_AUTH_URL"))

	mgmtHandler := generalmgmt.NewHandler(buf, "authentication", map[string]string{
		"PORT":         port,
		"DB_PATH":      os.Getenv("DB_PATH"),
		"TLS_PORT":     os.Getenv("TLS_PORT"),
		"MGMT_AUTH_URL": os.Getenv("MGMT_AUTH_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/authentication/general/", mgmtHandler)
	root.Handle("/", sysHandler)

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
			go startTLS(root, tlsPort, tlsCfg, "Authentication")
		}
	}

	slog.Info("Listening", "system", "Authentication", "port", port)
	log.Fatal(http.ListenAndServe(":"+port, root))
}

func startTLS(handler http.Handler, port string, tlsCfg *tls.Config, name string) {
	ln, err := tls.Listen("tcp", ":"+port, tlsCfg)
	if err != nil {
		log.Fatalf("[%s] TLS listen on :%s: %v", name, port, err)
	}
	slog.Info("Listening (HTTPS/mTLS)", "system", name, "port", port)
	if err := http.Serve(ln, handler); err != nil {
		log.Fatalf("[%s] TLS serve: %v", name, err)
	}
}
