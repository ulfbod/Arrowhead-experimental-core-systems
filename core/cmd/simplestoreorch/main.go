// Arrowhead Core – SimpleStoreServiceOrchestration entry point.
// Default port: 8084 (set PORT env var to override).
package main

import (
	"log"
	"net/http"
	"os"

	ssapi "arrowhead/core/internal/orchestration/simplestore/api"
	ssrepo "arrowhead/core/internal/orchestration/simplestore/repository"
	sssvc "arrowhead/core/internal/orchestration/simplestore/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	repo := ssrepo.NewMemoryRepository()
	orch := sssvc.NewSimpleStoreOrchestrator(repo)
	handler := ssapi.NewHandler(orch)

	log.Printf("[SimpleStoreOrchestration] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
