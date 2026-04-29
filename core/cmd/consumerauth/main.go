// Arrowhead Core – ConsumerAuthorization System entry point.
// Default port: 8082 (set PORT env var to override).
package main

import (
	"log"
	"net/http"
	"os"

	"arrowhead/core/internal/consumerauth/api"
	"arrowhead/core/internal/consumerauth/repository"
	"arrowhead/core/internal/consumerauth/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	repo := repository.NewMemoryRepository()
	svc := service.NewAuthService(repo)
	handler := api.NewHandler(svc)

	log.Printf("[ConsumerAuthorization] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
