package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/JCO-Digital/jman/internal/api"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/middleware"
)

// Version is injected by the build flags
var Version = "dev"

func main() {
	if err := config.Init(Version); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	port := os.Getenv("JMAN_API_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Register API routes from the internal/api package
	api.RegisterHandlers(mux, Version)

	// Wrap mux with middleware
	handler := middleware.LoggingMiddleware(
		middleware.CorsMiddleware(
			middleware.JsonMiddleware(mux),
		),
	)

	log.Printf("Starting jman-api (version: %s) on :%s", Version, port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
