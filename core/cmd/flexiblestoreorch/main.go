// Arrowhead Core – FlexibleStoreServiceOrchestration entry point.
// Default port: 8085 (set PORT env var to override).
package main

import (
	"log"
	"net/http"
	"os"

	fsapi "arrowhead/core/internal/orchestration/flexiblestore/api"
	fsrepo "arrowhead/core/internal/orchestration/flexiblestore/repository"
	fssvc "arrowhead/core/internal/orchestration/flexiblestore/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}

	repo := fsrepo.NewMemoryRepository()
	orch := fssvc.NewFlexibleStoreOrchestrator(repo)
	handler := fsapi.NewHandler(orch)

	log.Printf("[FlexibleStoreOrchestration] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
