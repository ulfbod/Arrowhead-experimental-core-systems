// Arrowhead Core – Authentication System entry point.
// Default port: 8081 (set PORT env var to override).
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"arrowhead/core/internal/authentication/api"
	"arrowhead/core/internal/authentication/repository"
	"arrowhead/core/internal/authentication/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	tokenDuration := 3600 * time.Second // 1 hour default

	repo := repository.NewMemoryRepository()
	svc := service.NewAuthService(repo, tokenDuration)
	handler := api.NewHandler(svc)

	log.Printf("[Authentication] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
