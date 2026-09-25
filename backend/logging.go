package main

import (
	"log"
	"net/http"
	"time"
)

// requestLogger wraps another handler with request logging.
// This kind of wrapper is called middleware.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. RECORD WHEN THE REQUEST STARTED
		start := time.Now()
		log.Printf("Request started: method=%s path=%s",
			r.Method, r.URL.Path)

		// 2. PASS THE REQUEST TO THE ROUTER
		// The router selects and runs the appropriate endpoint handler.
		next.ServeHTTP(w, r)

		// 3. RECORD WHEN THE HANDLER FINISHED
		log.Printf("Request finished: method=%s path=%s duration=%s",
			r.Method, r.URL.Path, time.Since(start))
	})
}
