package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListMessagesResponse struct {
	Messages   []Message `json:"messages"`
	NextCursor *int64    `json:"next_cursor"`
}

func listMessagesHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. REQUIRE AN AUTHENTICATED IDENTITY
		// Membership checks will use this identity in our next step.
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

		// 3. VALIDATE THE PAGE SIZE
		// An omitted limit defaults to 20.
		queryParams := r.URL.Query()
		limit := 20

		if values, present := queryParams["limit"]; present {
			// Reject empty or repeated parameters instead of guessing.
			if len(values) != 1 {
				http.Error(w, "Provide exactly one limit",
					http.StatusBadRequest)
				return
			}

			parsed, err := strconv.Atoi(values[0])
			if err != nil || parsed < 1 || parsed > 100 {
				http.Error(w, "limit must be an integer from 1 to 100",
					http.StatusBadRequest)
				return
			}
			limit = parsed
		}

		// 4. VALIDATE THE OPTIONAL CURSOR
		// nil means no cursor was supplied: fetch the latest page.
		var beforeID *int64

		if values, present := queryParams["before_id"]; present {
			if len(values) != 1 {
				http.Error(w, "Provide exactly one before_id",
					http.StatusBadRequest)
				return
			}

			parsed, err := strconv.ParseInt(values[0], 10, 64)
			if err != nil || parsed <= 0 {
				http.Error(w, "before_id must be a positive integer",
					http.StatusBadRequest)
				return
			}
			beforeID = &parsed
		}

		// 5. START A BOUNDED TRANSACTION
		// Keep membership valid while we read the requested page.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Printf("Begin message history transaction: %v", err)
			http.Error(w, "Could not load messages",
				http.StatusInternalServerError)
			return
		}

		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 3*time.Second,
			)
			defer cleanupCancel()

			if err := tx.Rollback(cleanupCtx); err != nil &&
				!errors.Is(err, pgx.ErrTxClosed) {
				log.Printf("Roll back message history: %v", err)
			}
		}()

		// 6. VERIFY AND LOCK MEMBERSHIP
		// A missing conversation and a nonmember receive the same response.
		var membershipID int64

		err = tx.QueryRow(ctx, `
			SELECT id
			FROM conversation_members
			WHERE conversation_id = $1 AND user_id = $2
			FOR SHARE
		`, conversationID, user.ID).Scan(&membershipID)

		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Conversation not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("Check history membership: %v", err)
			http.Error(w, "Could not load messages",
				http.StatusInternalServerError)
			return
		}

		// 7. FETCH ONE EXTRA MESSAGE TO DETECT ANOTHER PAGE
		// Both query forms use parameters for all client-supplied values.
		query := `
			SELECT id, conversation_id, sender_id, body, created_at
			FROM messages
			WHERE conversation_id = $1
			ORDER BY id DESC
			LIMIT $2
		`
		args := []any{conversationID, limit + 1}

		if beforeID != nil {
			query = `
				SELECT id, conversation_id, sender_id, body, created_at
				FROM messages
				WHERE conversation_id = $1 AND id < $2
				ORDER BY id DESC
				LIMIT $3
			`
			args = []any{conversationID, *beforeID, limit + 1}
		}

		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			log.Printf("Query message history: %v", err)
			http.Error(w, "Could not load messages",
				http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// 8. COLLECT THE RESULTS
		// An initialized empty slice becomes [] rather than null in JSON.
		messages := make([]Message, 0, limit+1)

		for rows.Next() {
			var message Message

			if err := rows.Scan(
				&message.ID,
				&message.ConversationID,
				&message.SenderID,
				&message.Body,
				&message.CreatedAt,
			); err != nil {
				log.Print("Read message history row failed")
				http.Error(w, "Could not load messages",
					http.StatusInternalServerError)
				return
			}

			messages = append(messages, message)
		}

		if err := rows.Err(); err != nil {
			log.Print("Iterate message history failed")
			http.Error(w, "Could not load messages",
				http.StatusInternalServerError)
			return
		}

		// Close the result set before committing the transaction.
		rows.Close()

		// 9. RELEASE THE MEMBERSHIP LOCK BEFORE WRITING THE RESPONSE
		if err := tx.Commit(ctx); err != nil {
			log.Printf("Commit message history transaction: %v", err)
			http.Error(w, "Could not load messages",
				http.StatusInternalServerError)
			return
		}

		// 10. BUILD THE PAGE AND OPTIONAL NEXT CURSOR
		var nextCursor *int64

		if len(messages) > limit {
			messages = messages[:limit]
			lastID := messages[len(messages)-1].ID
			nextCursor = &lastID
		}

		response := ListMessagesResponse{
			Messages:   messages,
			NextCursor: nextCursor,
		}

		// 11. RETURN THE PAGE
		// Authentication middleware already sets Cache-Control: no-store.
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Write message history response: %v", err)
		}

	}
}
