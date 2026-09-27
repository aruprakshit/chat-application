package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func logoutHandler(pool *pgxpool.Pool, secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		// 1. REVOKE THE SESSION IF A COOKIE IS PRESENT
		// Deleting a nonexistent session is harmless.
		// This makes repeated logout requests safe.
		cookie, err := r.Cookie("session")
		if err == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			_, err := pool.Exec(ctx, `
				DELETE FROM sessions
				WHERE token_hash = $1
			`, hashSessionToken(cookie.Value))

			if err != nil {
				log.Printf("Delete session: %v", err)
				http.Error(w, "Could not log out",
					http.StatusServiceUnavailable)
				return
			}
		}

		// 2. CLEAR THE BROWSER COOKIE
		// Use the same name and path as the login cookie.
		// A negative MaxAge tells the browser to delete it.
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(1, 0),
		})

		// 3. REPORT SUCCESS WITHOUT A RESPONSE BODY
		w.WriteHeader(http.StatusNoContent)
	}
}
