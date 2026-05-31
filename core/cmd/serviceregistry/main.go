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
	"log/slog"
	"net/http"
	"os"

	"arrowhead/core/internal/api"
	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/config"
	"arrowhead/core/internal/generalmgmt"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	cfg := config.Load()

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	// Legacy AH4-compatible handler (register/query/lookup/unregister).
	var repo repository.Repository
	var ah5Store repository.AH5StoreInterface
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		legacySQLite, err := repository.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[ServiceRegistry] open legacy database: %v", err)
		}
		repo = legacySQLite

		ah5SQLite, err := repository.NewAH5SQLiteStore(dbPath + ".ah5")
		if err != nil {
			log.Fatalf("[ServiceRegistry] open AH5 database: %v", err)
		}
		ah5Store = ah5SQLite
	} else {
		repo = repository.NewMemoryRepository()
		ah5Store = repository.NewAH5Store()
	}
	svc := service.NewRegistryService(repo)

	var blClient blclient.BlacklistClient = blclient.NopClient{}
	if blURL := os.Getenv("BLACKLIST_URL"); blURL != "" {
		blClient = blclient.NewHTTPClient(blURL, http.DefaultClient)
	}
	legacyHandler := api.NewHandler(svc, blClient)

	// AH5 discovery and management handler.
	ah5Svc := service.NewAH5RegistryService(ah5Store)
	ah5Handler := api.NewAH5Handler(ah5Svc, os.Getenv("SR_AUTH_URL"), os.Getenv("MGMT_AUTH_URL"), os.Getenv("REGISTER_AUTH_URL"))

	mgmtHandler := generalmgmt.NewHandler(buf, "serviceregistry", map[string]string{
		"PORT":              cfg.Port,
		"DB_PATH":           os.Getenv("DB_PATH"),
		"TLS_PORT":          os.Getenv("TLS_PORT"),
		"MGMT_AUTH_URL":     os.Getenv("MGMT_AUTH_URL"),
		"REGISTER_AUTH_URL": os.Getenv("REGISTER_AUTH_URL"),
	})

	mux := http.NewServeMux()
	// GeneralManagement — more specific than /serviceregistry/, so registered first.
	mux.Handle("/serviceregistry/general/", mgmtHandler)
	// AH5 routes are more specific and must be registered before the legacy
	// catch-all so that Go's ServeMux prefers them on longer path matches.
	mux.Handle("/serviceregistry/device-discovery/", ah5Handler)
	mux.Handle("/serviceregistry/system-discovery/", ah5Handler)
	mux.Handle("/serviceregistry/service-discovery/", ah5Handler)
	mux.Handle("/serviceregistry/mgmt/", ah5Handler)
	// Legacy catch-all for /serviceregistry/{register,query,lookup,unregister}.
	mux.Handle("/serviceregistry/", legacyHandler)
	mux.Handle("/health", legacyHandler)

	const distDir = "dashboard/dist"
	if info, err := os.Stat(distDir); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(distDir)))
		slog.Info("Dashboard available", "url", "http://localhost:"+cfg.Port+"/")
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

	slog.Info("Listening", "system", "ServiceRegistry", "port", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
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
