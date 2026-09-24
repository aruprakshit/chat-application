package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func usersHandler(pool *pgxpool.Pool) http.HandlerFunc {
	// Return an HTTP handler that shares the application's database pool.
	// The returned function remembers "pool" through a closure.
	return func(w http.ResponseWriter, r *http.Request) {

		// 1. LIMIT DATABASE WORK
		// Stop waiting after 3 seconds, or sooner if the request is canceled.
		// defer releases the timeout's resources when this handler returns.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		// 2. QUERY POSTGRESQL
		// Fetch at most 50 users in a predictable order.
		// The pool manages borrowing a connection for this query.
		rows, err := pool.Query(
			ctx,
			"SELECT id, username FROM users ORDER BY id LIMIT 50",
		)
		if err != nil {
			// Keep technical details in server logs.
			// Send a generic error to the client and stop processing.
			log.Printf("Query users: %v", err)
			http.Error(w, "Could not load users", http.StatusInternalServerError)
			return
		}

		// Ensure query resources are released, including on an early return.
		defer rows.Close()

		// 3. CONVERT DATABASE ROWS INTO GO VALUES
		// An empty slice becomes [] in JSON, rather than null.
		users := make([]User, 0)

		for rows.Next() {
			var user User

			// Scan copies columns into fields in SELECT order: id, username.
			// & gives Scan the addresses of the fields it should fill.
			if err := rows.Scan(&user.ID, &user.Username); err != nil {
				log.Printf("Read user: %v", err)
				http.Error(w, "Could not load users", http.StatusInternalServerError)
				return
			}

			users = append(users, user)
		}

		// 4. CHECK THAT ALL ROWS WERE READ SUCCESSFULLY
		// Next returns false both at the end and when an error occurs.
		// Check Err so we don't send a partial result as a success.
		if err := rows.Err(); err != nil {
			log.Printf("Iterate users: %v", err)
			http.Error(w, "Could not load users", http.StatusInternalServerError)
			return
		}

		// 5. SEND THE JSON RESPONSE
		// Set the content type before writing the response body.
		// Writing the body implicitly sends HTTP 200 unless set otherwise.
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(users); err != nil {
			// The response may already have started, so log the failure
			// instead of attempting to send a second error response.
			log.Printf("Write users response: %v", err)
		}
	}
}
