package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Conversation struct {
	ID        int64     `json:"id"`
	Title     *string   `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

func conversationsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. READ THE AUTHENTICATED USER
		// Middleware put this identity into the request context.
		user, ok := r.Context().Value(authContextKey{}).(User)
		if !ok {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// 2. FETCH ONLY THIS USER'S CONVERSATIONS
		// The membership filter enforces authorization inside the query.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		rows, err := pool.Query(ctx, `
			SELECT c.id, c.title, c.created_at
			FROM conversations AS c
			JOIN conversation_members AS cm
				ON cm.conversation_id = c.id
			WHERE cm.user_id = $1
			ORDER BY c.id DESC
			LIMIT 50
		`, user.ID)
		if err != nil {
			log.Printf("List conversations: %v", err)
			http.Error(w, "Could not load conversations",
				http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// 3. COLLECT DATABASE ROWS INTO RESPONSE VALUES
		conversations := make([]Conversation, 0)

		for rows.Next() {
			var conversation Conversation

			if err := rows.Scan(
				&conversation.ID,
				&conversation.Title,
				&conversation.CreatedAt,
			); err != nil {
				log.Printf("Read conversation: %v", err)
				http.Error(w, "Could not load conversations",
					http.StatusInternalServerError)
				return
			}

			conversations = append(conversations, conversation)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Iterate conversations: %v", err)
			http.Error(w, "Could not load conversations",
				http.StatusInternalServerError)
			return
		}

		// 4. WRITE THE CONVERSATIONS AS JSON
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(conversations); err != nil {
			log.Printf("Write conversations response: %v", err)
		}
	}
}
