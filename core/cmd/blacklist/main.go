// Arrowhead Core – Blacklist entry point.
// Default port: 8087 (set PORT env var to override).
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	blapi "arrowhead/core/internal/blacklist/api"
	blrepo "arrowhead/core/internal/blacklist/repository"
	blsvc "arrowhead/core/internal/blacklist/service"
	"arrowhead/core/internal/generalmgmt"
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

	slog.Info("Listening", "system", "Blacklist", "port", port)
	log.Fatal(http.ListenAndServe(":"+port, root))
}
