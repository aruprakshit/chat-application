package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

func conversationSocketHandler(pool *pgxpool.Pool, allowedOrigin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. READ THE IDENTITY VERIFIED BY AUTHENTICATION MIDDLEWARE
		user, ok := r.Context().Value(authContextKey{}).(User)
		if !ok {
			http.Error(w, "Authentication required",
				http.StatusUnauthorized)
			return
		}

		// 2. VALIDATE THE CONVERSATION ID
		conversationID, err := strconv.ParseInt(
			r.PathValue("id"), 10, 64,
		)
		if err != nil || conversationID <= 0 {
			http.Error(w, "Conversation ID must be a positive integer",
				http.StatusBadRequest)
			return
		}

		// 3. CHECK MEMBERSHIP WITH A SHORT DATABASE DEADLINE
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)

		var isMember bool
		err = pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM conversation_members
				WHERE conversation_id = $1 AND user_id = $2
			)
		`, conversationID, user.ID).Scan(&isMember)

		// Finish this database operation before starting connection work.
		cancel()

		if err != nil {
			log.Printf("Check WebSocket membership: %v", err)
			http.Error(w, "Could not verify conversation access",
				http.StatusServiceUnavailable)
			return
		}

		if !isMember {
			http.Error(w, "Conversation not found", http.StatusNotFound)
			return
		}

		// 4. UPGRADE THE AUTHORIZED REQUEST TO WEBSOCKET
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{allowedOrigin},
		})
		if err != nil {
			// Accept already writes the HTTP error response.
			log.Printf("Accept conversation WebSocket: %v", err)
			return
		}
		defer conn.CloseNow()

		// 5. TEMPORARY CHECKPOINT: COMPLETE A NORMAL CLOSE HANDSHAKE
		// Live subscriptions and message delivery come in the next lesson.
		if err := conn.Close(
			websocket.StatusNormalClosure,
			"Connection checks passed",
		); err != nil {
			log.Printf("Close conversation WebSocket: %v", err)
		}
	}
}
