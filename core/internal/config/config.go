package config

import "os"

// Config holds runtime configuration for the Service Registry.
type Config struct {
	Port string
}

// Load reads configuration from environment variables, applying defaults.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return Config{Port: port}
}
