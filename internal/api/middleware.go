package api

import (
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
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

// SecurityHeadersMiddleware adds standard security headers to all responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// CorsMiddleware adds CORS headers based on allowed origins in configuration.
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigins := config.Cfg.AllowedOrigins

		allow := false
		if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			allow = true
		} else if origin != "" {
			w.Header().Add("Vary", "Origin")
			for _, o := range allowedOrigins {
				if o == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					allow = true
					break
				}
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			if !allow && origin != "" && len(allowedOrigins) > 0 && allowedOrigins[0] != "*" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
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

// levelToInt converts a UserLevel to an integer for comparison.
func levelToInt(l config.UserLevel) int {
	switch l {
	case config.LevelExecute:
		return 2
	case config.LevelEdit:
		return 1
	case config.LevelBasic:
		return 0
	default:
		return 0
	}
}

// RequireLevel returns a middleware that checks if the authenticated user has at least the required level.
func RequireLevel(minLevel config.UserLevel) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetAuthClaims(r.Context())
			if claims == nil {
				WriteError(w, http.StatusUnauthorized, "Authentication required")
				return
			}

			if levelToInt(claims.Level) < levelToInt(minLevel) {
				WriteError(w, http.StatusForbidden, "Insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
