// Arrowhead Core – FlexibleStoreServiceOrchestration entry point.
// Default port: 8085 (set PORT env var to override).
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"arrowhead/core/internal/generalmgmt"
	fsapi "arrowhead/core/internal/orchestration/flexiblestore/api"
	fsrepo "arrowhead/core/internal/orchestration/flexiblestore/repository"
	fssvc "arrowhead/core/internal/orchestration/flexiblestore/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var repo fsrepo.Repository
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		sqliteRepo, err := fsrepo.NewSQLiteRepository(dbPath)
		if err != nil {
			log.Fatalf("[FlexibleStoreOrchestration] open database: %v", err)
		}
		repo = sqliteRepo
	} else {
		repo = fsrepo.NewMemoryRepository()
	}
	orch := fssvc.NewFlexibleStoreOrchestrator(repo)
	sysHandler := fsapi.NewHandler(orch, os.Getenv("MGMT_AUTH_URL"))

	mgmtHandler := generalmgmt.NewHandler(buf, "serviceorchestration/orchestration", map[string]string{
		"PORT":         port,
		"DB_PATH":      os.Getenv("DB_PATH"),
		"MGMT_AUTH_URL": os.Getenv("MGMT_AUTH_URL"),
	})

	root := http.NewServeMux()
	root.Handle("/serviceorchestration/orchestration/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	slog.Info("Listening", "system", "FlexibleStoreOrchestration", "port", port)
	log.Fatal(http.ListenAndServe(":"+port, root))
}
