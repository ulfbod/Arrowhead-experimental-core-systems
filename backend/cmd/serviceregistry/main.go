package main

import (
	"log"
	"mineio/internal/api"
	"mineio/internal/config"
	"mineio/internal/repository"
	"mineio/internal/service"
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
