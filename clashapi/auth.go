package clashapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func authMiddleware(secret string, next http.Handler) http.Handler {
	if secret == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		if t := r.URL.Query().Get("token"); t != "" {
			if subtle.ConstantTimeCompare([]byte(t), []byte(secret)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="clash"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func corsMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	if allowedOrigins == "" {
		allowedOrigins = "*"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowedOrigins == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					for _, o := range strings.Split(allowedOrigins, ",") {
						if strings.TrimSpace(o) == origin {
							w.Header().Set("Access-Control-Allow-Origin", origin)
							break
						}
					}
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "3600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}