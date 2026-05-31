// Arrowhead Core — Translation Manager entry point.
package main

import (
	"log"
	"net/http"
	"os"

	tmapi "arrowhead/core/internal/translationmgr/api"
	"arrowhead/core/internal/translationmgr/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}

	svc := service.NewTranslationService()
	handler := tmapi.NewHandler(svc)

	log.Printf("[TranslationManager] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
