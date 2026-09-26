package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func readinessHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. BOUND THE CHECK
		// A readiness request should not wait indefinitely for the database.
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		// 2. VERIFY DATABASE CONNECTIVITY
		// Ping borrows a connection and checks that PostgreSQL responds.
		if err := pool.Ping(ctx); err != nil {
			log.Printf("Readiness check failed: %v", err)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Not ready", http.StatusServiceUnavailable)
			return
		}

		// 3. REPORT SUCCESS
		// Reuse our existing JSON health response after the check passes.
		w.Header().Set("Cache-Control", "no-store")
		healthHandler(w, r)
	}
}
