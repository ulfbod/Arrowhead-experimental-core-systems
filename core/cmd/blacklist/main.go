// Arrowhead Core – Blacklist entry point.
// Default port: 8087 (set PORT env var to override).
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	blapi "arrowhead/core/internal/blacklist/api"
	blrepo "arrowhead/core/internal/blacklist/repository"
	blsvc "arrowhead/core/internal/blacklist/service"
	"arrowhead/core/internal/generalmgmt"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8087"
	}

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var repo blrepo.Repository
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		sqliteRepo, err := blrepo.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[Blacklist] open database: %v", err)
		}
		repo = sqliteRepo
	} else {
		repo = blrepo.NewMemoryRepository()
	}

	svc := blsvc.NewBlacklistService(repo)
	sysHandler := blapi.NewHandler(svc, os.Getenv("BLACKLIST_AUTH_URL"), os.Getenv("MGMT_AUTH_URL"))

	mgmtHandler := generalmgmt.NewHandler(buf, "blacklist", map[string]string{
		"PORT":         port,
		"DB_PATH":      os.Getenv("DB_PATH"),
		"MGMT_AUTH_URL": os.Getenv("MGMT_AUTH_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/blacklist/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	// Start background purge goroutine (G50).
	// BLACKLIST_PURGE_INTERVAL_SECONDS=0 disables auto-purge; default 3600.
	purgeIntervalStr := os.Getenv("BLACKLIST_PURGE_INTERVAL_SECONDS")
	purgeInterval := 3600
	if purgeIntervalStr != "" {
		if v, err := strconv.Atoi(purgeIntervalStr); err == nil {
			purgeInterval = v
		}
	}
	if purgeInterval > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(purgeInterval) * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				n := svc.PurgeExpired()
				if n > 0 {
					slog.Info("Purged expired blacklist entries", "count", n)
				}
			}
		}()
	}

	tlsCfg, err := tlsutil.LoadServerTLSConfig(
		os.Getenv("TLS_CERT_FILE"),
		os.Getenv("TLS_KEY_FILE"),
		os.Getenv("TLS_CA_FILE"),
	)
	if err != nil {
		log.Fatalf("[Blacklist] TLS config: %v", err)
	}
	httpsOnly := os.Getenv("HTTPS_ONLY") == "true"
	tlsAddr := ""
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsAddr = ":" + tlsPort
	}

	slog.Info("Listening", "system", "Blacklist", "port", port)
	log.Fatal(tlsutil.ServeHTTPS(":"+port, tlsAddr, root, tlsCfg, httpsOnly))
}
