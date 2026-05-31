// Arrowhead Core – Certificate Authority entry point.
// Default port: 8086 (set PORT env var to override).
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"arrowhead/core/internal/ca/api"
	carepo "arrowhead/core/internal/ca/repository"
	"arrowhead/core/internal/ca/service"
	"arrowhead/core/internal/generalmgmt"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}
	certDuration := 24 * time.Hour

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var repo carepo.Repository
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		sqliteRepo, err := carepo.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[CA] open database: %v", err)
		}
		repo = sqliteRepo
	} else {
		repo = carepo.NewMemoryRepository()
	}
	svc, err := service.NewCAServiceWithRepo(certDuration, repo)
	if err != nil {
		log.Fatalf("[CA] Failed to initialise CA: %v", err)
	}
	sysHandler := api.NewHandler(svc)

	mgmtHandler := generalmgmt.NewHandler(buf, "ca", map[string]string{
		"PORT":         port,
		"DB_PATH":      os.Getenv("DB_PATH"),
		"MGMT_AUTH_URL": os.Getenv("MGMT_AUTH_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/ca/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	slog.Info("Listening", "system", "CA", "port", port)
	log.Fatal(http.ListenAndServe(":"+port, root))
}
