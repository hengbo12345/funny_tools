package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// checkToken validates if the request has the correct token
func checkToken(r *http.Request) bool {
	if tokenFlag == "" {
		return true // token not required
	}

	// 1. Check Authorization Header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Accept both "Bearer <token>" and direct "<token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			if parts[1] == tokenFlag {
				return true
			}
		} else if authHeader == tokenFlag {
			return true
		}
	}

	// 2. Check Custom Header
	customHeader := r.Header.Get("X-GPS-Token")
	if customHeader == tokenFlag {
		return true
	}

	// 3. Check URL Query parameter (needed for EventSource SSE logs)
	queryToken := r.URL.Query().Get("token")
	if queryToken == tokenFlag {
		return true
	}

	return false
}

// withAuth is a middleware wrapper for authenticated API routes
func withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Unauthorized: Invalid or missing token",
			})
			return
		}
		handler(w, r)
	}
}
