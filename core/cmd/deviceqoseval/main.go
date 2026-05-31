// Arrowhead Core — Device QoS Evaluator entry point.
package main

import (
	"log"
	"os"

	qosapi "arrowhead/core/internal/deviceqoseval/api"
	"arrowhead/core/internal/deviceqoseval/repository"
	"arrowhead/core/internal/deviceqoseval/service"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	repo := repository.NewMemoryRepository()
	eval := service.NewEvaluator(repo)
	handler := qosapi.NewHandler(eval)

	tlsCfg, err := tlsutil.LoadServerTLSConfig(
		os.Getenv("TLS_CERT_FILE"),
		os.Getenv("TLS_KEY_FILE"),
		os.Getenv("TLS_CA_FILE"),
	)
	if err != nil {
		log.Fatalf("[DeviceQoSEvaluator] TLS config: %v", err)
	}
	httpsOnly := os.Getenv("HTTPS_ONLY") == "true"
	tlsAddr := ""
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsAddr = ":" + tlsPort
	}

	log.Printf("[DeviceQoSEvaluator] Listening on :%s", port)
	log.Fatal(tlsutil.ServeHTTPS(":"+port, tlsAddr, handler, tlsCfg, httpsOnly))
}
