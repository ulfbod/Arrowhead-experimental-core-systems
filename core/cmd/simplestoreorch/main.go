// Arrowhead Core – SimpleStoreServiceOrchestration entry point.
// Default port: 8084 (set PORT env var to override).
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"arrowhead/core/internal/generalmgmt"
	ssapi "arrowhead/core/internal/orchestration/simplestore/api"
	ssrepo "arrowhead/core/internal/orchestration/simplestore/repository"
	sssvc "arrowhead/core/internal/orchestration/simplestore/service"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var repo ssrepo.Repository
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		sqliteRepo, err := ssrepo.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[SimpleStoreOrchestration] open database: %v", err)
		}
		repo = sqliteRepo
	} else {
		repo = ssrepo.NewMemoryRepository()
	}
	orch := sssvc.NewSimpleStoreOrchestrator(repo)
	sysHandler := ssapi.NewHandler(orch, os.Getenv("MGMT_AUTH_URL"))

	mgmtHandler := generalmgmt.NewHandler(buf, "serviceorchestration/orchestration", map[string]string{
		"PORT":         port,
		"DB_PATH":      os.Getenv("DB_PATH"),
		"MGMT_AUTH_URL": os.Getenv("MGMT_AUTH_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/serviceorchestration/orchestration/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	tlsCfg, err := tlsutil.LoadServerTLSConfig(
		os.Getenv("TLS_CERT_FILE"),
		os.Getenv("TLS_KEY_FILE"),
		os.Getenv("TLS_CA_FILE"),
	)
	if err != nil {
		log.Fatalf("[SimpleStoreOrchestration] TLS config: %v", err)
	}
	httpsOnly := os.Getenv("HTTPS_ONLY") == "true"
	tlsAddr := ""
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsAddr = ":" + tlsPort
	}

	slog.Info("Listening", "system", "SimpleStoreOrchestration", "port", port)
	log.Fatal(tlsutil.ServeHTTPS(":"+port, tlsAddr, root, tlsCfg, httpsOnly))
}
