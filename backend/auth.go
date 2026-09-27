package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A private key type avoids collisions with other context values.
type authContextKey struct{}

func requireAuth(pool *pgxpool.Pool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. READ AND VALIDATE THE COOKIE
		// Authenticated responses should not be cached.
		w.Header().Set("Cache-Control", "no-store")

		cookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Our token helper encodes exactly 32 bytes as unpadded base64.
		// Reject malformed tokens before borrowing a database connection.
		if len(cookie.Value) != 43 {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		decoded, err := base64.RawURLEncoding.Strict().DecodeString(cookie.Value)
		if err != nil || len(decoded) != 32 {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// 2. LOOK UP AN UNEXPIRED SESSION
		// Hash the exact cookie text, just as we did during login.
		tokenHash := hashSessionToken(cookie.Value)

		queryCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)

		var user User
		err = pool.QueryRow(queryCtx, `
			SELECT u.id, u.username
			FROM sessions AS s
			JOIN users AS u ON u.id = s.user_id
			WHERE s.token_hash = $1
			  AND s.expires_at > CURRENT_TIMESTAMP
		`, tokenHash).Scan(&user.ID, &user.Username)

		cancel()

		if errors.Is(err, pgx.ErrNoRows) {
			// Unknown and expired sessions get the same response.
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		if err != nil {
			// A database failure is not evidence of invalid credentials.
			log.Printf("Look up session: %v", err)
			http.Error(w, "Could not verify session",
				http.StatusServiceUnavailable)
			return
		}

		// 3. ATTACH THE VERIFIED USER TO THIS REQUEST
		// Use the original request context, not the canceled query context.
		ctx := context.WithValue(r.Context(), authContextKey{}, user)

		// 4. CONTINUE TO THE PROTECTED HANDLER
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	// READ THE USER VERIFIED BY THE MIDDLEWARE
	// The type assertion checks that the context value is a User.
	user, ok := r.Context().Value(authContextKey{}).(User)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// WRITE THE USER'S ID AND USERNAME TO THE HTTP RESPONSE AS JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(user); err != nil {
		log.Printf("Write current user response: %v", err)
	}
}
