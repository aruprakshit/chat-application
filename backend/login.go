package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func loginHandler(pool *pgxpool.Pool, secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. VALIDATE THE REQUEST FORMAT
		// Login responses should not be cached.
		w.Header().Set("Cache-Control", "no-store")

		if !requireJSONContentType(w, r) {
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		defer r.Body.Close()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input LoginRequest
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Invalid login request", http.StatusBadRequest)
			return
		}

		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "Body must contain exactly one JSON value",
				http.StatusBadRequest)
			return
		}

		if !usernamePattern.MatchString(input.Username) ||
			len(input.Password) == 0 || len(input.Password) > 72 {
			http.Error(w, "Invalid username or password",
				http.StatusUnauthorized)
			return
		}

		// 2. FETCH THE USER'S CREDENTIALS
		// COALESCE turns a NULL hash from an old learning account
		// into an empty string, which cannot authenticate.
		queryCtx, queryCancel := context.WithTimeout(
			r.Context(), 3*time.Second,
		)

		var userID int64
		var passwordHash string

		err := pool.QueryRow(queryCtx, `
			SELECT id, COALESCE(password_hash, '')
			FROM users
			WHERE username = $1
		`, input.Username).Scan(&userID, &passwordHash)

		// Release this operation's timeout before doing CPU work.
		queryCancel()

		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Invalid username or password",
				http.StatusUnauthorized)
			return
		}
		if err != nil {
			log.Printf("Look up login user: %v", err)
			http.Error(w, "Could not log in",
				http.StatusInternalServerError)
			return
		}

		// 3. VERIFY THE PASSWORD
		// Use bcrypt's comparison, not a newly generated hash.
		// Never log credentials or password hashes.
		if err := bcrypt.CompareHashAndPassword(
			[]byte(passwordHash), []byte(input.Password),
		); err != nil {
			http.Error(w, "Invalid username or password",
				http.StatusUnauthorized)
			return
		}

		// 4. CREATE AND STORE THE SESSION
		// The raw token goes to the browser; only its hash goes to SQL.
		token, tokenHash := newSessionToken()

		insertCtx, insertCancel := context.WithTimeout(
			r.Context(), 3*time.Second,
		)
		defer insertCancel()

		var expiresAt time.Time

		err = pool.QueryRow(insertCtx, `
			INSERT INTO sessions (token_hash, user_id, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '24 hours')
			RETURNING expires_at
		`, tokenHash, userID).Scan(&expiresAt)

		if err != nil {
			log.Printf("Create session: %v", err)
			http.Error(w, "Could not log in",
				http.StatusInternalServerError)
			return
		}

		// 5. SET THE COOKIE AFTER THE SESSION IS STORED
		// HttpOnly prevents JavaScript from reading the cookie.
		// Secure requires HTTPS when enabled.
		// SameSite=Lax restricts cookie sending on cross-site requests.
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
			Expires:  expiresAt,
		})

		// Success with no response body; the cookie carries the token.
		w.WriteHeader(http.StatusNoContent)
	}
}
