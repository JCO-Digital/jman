package api

import (
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/verb"
)

// jsonMiddleware ensures Content-Type is set, except if an error was already returned in plain text (though we use plain text for simplicity in errors above, we can set default to json).
// Actually, it's better to just ensure application/json is the default.
func JsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds basic CORS headers
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs the incoming HTTP requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(ww, r)
		verb.LogPrintf(verb.Normal, "%s %s %d %s", r.Method, r.URL.Path, ww.statusCode, time.Since(start))
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
