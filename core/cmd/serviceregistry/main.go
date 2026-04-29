// Arrowhead Core – Service Registry entry point.
//
// DO NOT MODIFY FOR EXPERIMENTS.
package main

import (
	"log"
	"net/http"
	"os"

	"arrowhead/core/internal/api"
	"arrowhead/core/internal/config"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
)

func main() {
	cfg := config.Load()

	repo := repository.NewMemoryRepository()
	svc := service.NewRegistryService(repo)
	apiHandler := api.NewHandler(svc)

	mux := http.NewServeMux()
	mux.Handle("/serviceregistry/", apiHandler)
	mux.Handle("/health", apiHandler)

	const distDir = "dashboard/dist"
	if info, err := os.Stat(distDir); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(distDir)))
		log.Printf("[ServiceRegistry] Dashboard available at http://localhost:%s/", cfg.Port)
	}

	log.Printf("[ServiceRegistry] Listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
