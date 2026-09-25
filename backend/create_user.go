package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile the username rule once, rather than on every request.
var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,50}$`)

type CreateUserRequest struct {
	Username string `json:"username"`
}

func createUserHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. CHECK THE REQUEST FORMAT
		// ParseMediaType also accepts application/json; charset=utf-8.
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" {
			http.Error(w, "Content-Type must be application/json",
				http.StatusUnsupportedMediaType)
			return
		}

		// 2. READ A SMALL, STRICT JSON BODY
		// Cap input at 1 KiB and reject unexpected fields.
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		defer r.Body.Close()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input CreateUserRequest
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Expected a small JSON object with a username",
				http.StatusBadRequest)
			return
		}

		// Require the body to end after the first JSON value.
		// This rejects input such as two objects placed side by side.
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "Body must contain exactly one JSON value",
				http.StatusBadRequest)
			return
		}

		// 3. VALIDATE THE USERNAME
		// Missing, empty, uppercase, or unsupported characters fail here.
		if !usernamePattern.MatchString(input.Username) {
			http.Error(w,
				"Username must be 3-50 lowercase letters, digits, or underscores",
				http.StatusBadRequest)
			return
		}

		// 4. INSERT WITH A DATABASE DEADLINE
		// $1 keeps user input separate from SQL instructions.
		// PostgreSQL's unique constraint handles competing inserts safely.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var user User
		err = pool.QueryRow(ctx, `
			INSERT INTO users (username)
			VALUES ($1)
			ON CONFLICT (username) DO NOTHING
			RETURNING id, username
		`, input.Username).Scan(&user.ID, &user.Username)

		// 5. TRANSLATE DATABASE RESULTS INTO HTTP RESPONSES
		// A duplicate skips the insert, so RETURNING produces no row.
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Username already exists", http.StatusConflict)
			return
		}

		if err != nil {
			log.Printf("Create user: %v", err)
			http.Error(w, "Could not create user",
				http.StatusInternalServerError)
			return
		}

		// 6. RETURN THE CREATED USER
		// Set headers before the status, and the status before the body.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(user); err != nil {
			log.Printf("Write created user response: %v", err)
		}
	}
}
