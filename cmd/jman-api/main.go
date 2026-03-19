package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/JCO-Digital/jman/internal/api"
	"github.com/JCO-Digital/jman/internal/config"
)

func main() {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	port := os.Getenv("JMAN_API_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Register API routes from the internal/api package
	api.RegisterHandlers(mux, config.AppVersion)

	// Wrap mux with middleware
	handler := api.LoggingMiddleware(
		api.CorsMiddleware(
			api.JsonMiddleware(mux),
		),
	)

	log.Printf("Starting jman-api (version: %s) on :%s", config.AppVersion, port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
