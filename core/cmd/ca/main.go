// Arrowhead Core – Certificate Authority entry point.
// Default port: 8086 (set PORT env var to override).
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"arrowhead/core/internal/ca/api"
	"arrowhead/core/internal/ca/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}
	certDuration := 24 * time.Hour

	svc, err := service.NewCAService(certDuration)
	if err != nil {
		log.Fatalf("[CA] Failed to initialise CA: %v", err)
	}
	handler := api.NewHandler(svc)

	log.Printf("[CA] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
