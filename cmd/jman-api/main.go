package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
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

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":%q}`, Version)
	})

	mux.HandleFunc("GET /api/plugins", func(w http.ResponseWriter, r *http.Request) {
		var plugins []models.WPPlugin
		if err := cache.ReadJSONCache("plugins", &plugins); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired: %v. Run 'jman fetch plugins' to fetch data."}`, err), http.StatusNotFound)
			return
		}
		writeJSON(w, plugins)
	})

	mux.HandleFunc("GET /api/servers", func(w http.ResponseWriter, r *http.Request) {
		var servers []models.Server
		if err := cache.ReadJSONCache("servers", &servers); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired: %v. Run 'jman fetch servers' to fetch data."}`, err), http.StatusNotFound)
			return
		}
		writeJSON(w, servers)
	})

	mux.HandleFunc("GET /api/sites", func(w http.ResponseWriter, r *http.Request) {
		var sites []models.Site
		if err := cache.ReadJSONCache("sites", &sites); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired: %v. Run 'jman fetch sites' to fetch data."}`, err), http.StatusNotFound)
			return
		}
		writeJSON(w, sites)
	})

	mux.HandleFunc("GET /api/vulns", func(w http.ResponseWriter, r *http.Request) {
		plugin := r.URL.Query().Get("plugin")
		if plugin == "" {
			http.Error(w, `{"error":"Missing required query parameter: plugin"}`, http.StatusBadRequest)
			return
		}

		var vulnData models.VulnResponse
		filename := fmt.Sprintf("vulnerabilities/%s", plugin)
		if err := cache.ReadJSONCache(filename, &vulnData); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired for plugin %q: %v. Run 'jman vuln %s' to fetch data."}`, plugin, err, plugin), http.StatusNotFound)
			return
		}
		writeJSON(w, vulnData)
	})

	handler := loggingMiddleware(corsMiddleware(jsonMiddleware(mux)))

	log.Printf("Starting jman-api (version: %s) on :%s", Version, port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON: %v", err)
	}
}

// jsonMiddleware ensures Content-Type is set, except if an error was already returned in plain text (though we use plain text for simplicity in errors above, we can set default to json).
// Actually, it's better to just ensure application/json is the default.
func jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds basic CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs the incoming HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, ww.statusCode, time.Since(start))
	})
}

// responseWriter captures the status code for logging
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
