// Arrowhead Core — Device QoS Evaluator entry point.
package main

import (
	"log"
	"net/http"
	"os"

	qosapi "arrowhead/core/internal/deviceqoseval/api"
	"arrowhead/core/internal/deviceqoseval/repository"
	"arrowhead/core/internal/deviceqoseval/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	repo := repository.NewMemoryRepository()
	eval := service.NewEvaluator(repo)
	handler := qosapi.NewHandler(eval)

	log.Printf("[DeviceQoSEvaluator] Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
