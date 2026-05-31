// Arrowhead Core — Translation Manager entry point.
package main

import (
	"log"
	"os"

	tmapi "arrowhead/core/internal/translationmgr/api"
	"arrowhead/core/internal/translationmgr/service"
	"arrowhead/core/internal/tlsutil"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}

	svc := service.NewTranslationService()
	handler := tmapi.NewHandler(svc)

	tlsCfg, err := tlsutil.LoadServerTLSConfig(
		os.Getenv("TLS_CERT_FILE"),
		os.Getenv("TLS_KEY_FILE"),
		os.Getenv("TLS_CA_FILE"),
	)
	if err != nil {
		log.Fatalf("[TranslationManager] TLS config: %v", err)
	}
	httpsOnly := os.Getenv("HTTPS_ONLY") == "true"
	tlsAddr := ""
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsAddr = ":" + tlsPort
	}

	log.Printf("[TranslationManager] Listening on :%s", port)
	log.Fatal(tlsutil.ServeHTTPS(":"+port, tlsAddr, handler, tlsCfg, httpsOnly))
}
