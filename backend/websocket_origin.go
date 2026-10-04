package main

import "net/http"

func requireWebSocketOrigin(
	allowedOrigin string,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. REQUIRE ONE EXPLICITLY ALLOWED BROWSER ORIGIN
		// Missing configuration rejects requests rather than allowing all.
		origins := r.Header.Values("Origin")

		if allowedOrigin == "" ||
			len(origins) != 1 ||
			origins[0] != allowedOrigin {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// 2. CONTINUE TO AUTHENTICATION
		next.ServeHTTP(w, r)
	})
}
