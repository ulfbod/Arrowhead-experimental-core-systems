// Arrowhead Core – Service Registry entry point.
//
// DO NOT MODIFY FOR EXPERIMENTS.
// This binary is the stable core service. Experimental extensions belong in
// experiments/ and must communicate with this process via HTTP only.
package main

import (
	"log"
	"arrowhead/serviceregistry/internal/api"
	"arrowhead/serviceregistry/internal/config"
	"arrowhead/serviceregistry/internal/repository"
	"arrowhead/serviceregistry/internal/service"
	"net/http"
)

func main() {
	cfg := config.Load()

	repo := repository.NewMemoryRepository()
	svc := service.NewRegistryService(repo)
	handler := api.NewHandler(svc)

	log.Printf("[ServiceRegistry] Listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, handler))
}
