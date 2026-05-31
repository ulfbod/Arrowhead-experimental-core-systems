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
	"crypto/tls"
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

	var blClient blclient.BlacklistClient = blclient.NopClient{}
	if blURL := os.Getenv("BLACKLIST_URL"); blURL != "" {
		blClient = blclient.NewHTTPClient(blURL, http.DefaultClient)
	}
	sysHandler := api.NewHandler(svc, os.Getenv("MGMT_AUTH_URL"), blClient)

	mgmtHandler := generalmgmt.NewHandler(buf, "authorization", map[string]string{
		"PORT":          port,
		"DB_PATH":       os.Getenv("DB_PATH"),
		"TLS_PORT":      os.Getenv("TLS_PORT"),
		"MGMT_AUTH_URL": os.Getenv("MGMT_AUTH_URL"),
		"BLACKLIST_URL":  os.Getenv("BLACKLIST_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/authorization/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	// Optional TLS listener on TLS_PORT.
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsCfg, err := tlsutil.LoadServerTLSConfig(
			os.Getenv("TLS_CERT_FILE"),
			os.Getenv("TLS_KEY_FILE"),
			os.Getenv("TLS_CA_FILE"),
		)
		if err != nil {
			log.Fatalf("[ConsumerAuthorization] TLS config: %v", err)
		}
		if tlsCfg != nil {
			go startTLS(root, tlsPort, tlsCfg, "ConsumerAuthorization")
		}
	}

	slog.Info("Listening", "system", "ConsumerAuthorization", "port", port)
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
