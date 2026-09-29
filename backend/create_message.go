package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateMessageRequest struct {
	Body string `json:"body"`
}

// Message contains the public fields returned after a message is stored.
type Message struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	SenderID       int64     `json:"sender_id"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

func createMessageHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. REQUIRE AN AUTHENTICATED IDENTITY
		// Middleware identifies the sender; the client cannot choose one.
		user, ok := r.Context().Value(authContextKey{}).(User)
		if !ok {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// 2. PARSE THE CONVERSATION ID FROM THE URL
		// PathValue reads the {id} captured by our router pattern.
		conversationID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || conversationID <= 0 {
			http.Error(w, "Conversation ID must be a positive integer",
				http.StatusBadRequest)
			return
		}

		// 3. REQUIRE JSON INPUT
		if !requireJSONContentType(w, r) {
			return
		}

		// 4. READ ONE JSON OBJECT WITHIN THE REQUEST SIZE LIMIT
		// 32 << 10 means 32 * 1024 bytes: 32 KiB.
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		defer r.Body.Close()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input CreateMessageRequest
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Expected a JSON object with a message body",
				http.StatusBadRequest)
			return
		}

		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "Body must contain exactly one JSON value",
				http.StatusBadRequest)
			return
		}

		// 5. VALIDATE MESSAGE CONTENT WITHOUT MODIFYING IT
		// Count Unicode code points, not UTF-8 bytes.
		// TrimSpace is used only for checking whether content is blank.
		length := utf8.RuneCountInString(input.Body)
		if length < 1 || length > 4000 ||
			strings.TrimSpace(input.Body) == "" {
			http.Error(w,
				"Message must contain 1-4000 characters and cannot be blank",
				http.StatusBadRequest)
			return
		}

		// 6. START A DATABASE TRANSACTION
		// Membership verification and insertion share one connection and deadline.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Printf("Begin message transaction: %v", err)
			http.Error(w, "Could not create message",
				http.StatusInternalServerError)
			return
		}

		// Clean up on any early return.
		// A fresh context allows rollback even if the request has timed out.
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 3*time.Second,
			)
			defer cleanupCancel()

			if err := tx.Rollback(cleanupCtx); err != nil &&
				!errors.Is(err, pgx.ErrTxClosed) {
				log.Printf("Roll back message: %v", err)
			}
		}()

		// 7. VERIFY AND LOCK THE SENDER'S MEMBERSHIP
		// FOR SHARE prevents this membership from being updated or deleted
		// until our transaction ends. Other senders can still read it.
		var membershipID int64

		err = tx.QueryRow(ctx, `
			SELECT id
			FROM conversation_members
			WHERE conversation_id = $1 AND user_id = $2
			FOR SHARE
		`, conversationID, user.ID).Scan(&membershipID)

		if errors.Is(err, pgx.ErrNoRows) {
			// Use the same response for a missing conversation and a nonmember.
			// We do not reveal whether someone else's conversation exists.
			http.Error(w, "Conversation not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("Check message membership: %v", err)
			http.Error(w, "Could not create message",
				http.StatusInternalServerError)
			return
		}

		// 8. INSERT THE MESSAGE USING THE VERIFIED SENDER
		// Preserve the body exactly as submitted.
		// PostgreSQL generates the message ID and creation timestamp.
		var message Message

		err = tx.QueryRow(ctx, `
			INSERT INTO messages (conversation_id, sender_id, body)
			VALUES ($1, $2, $3)
			RETURNING id, conversation_id, sender_id, body, created_at
		`, conversationID, user.ID, input.Body).Scan(
			&message.ID,
			&message.ConversationID,
			&message.SenderID,
			&message.Body,
			&message.CreatedAt,
		)
		if err != nil {
			// Avoid logging database error details that could contain message text.
			log.Print("Insert message failed")
			http.Error(w, "Could not create message",
				http.StatusInternalServerError)
			return
		}

		// 9. COMMIT BEFORE REPORTING SUCCESS
		// RETURNING gives us values, but the write is not final until commit.
		if err := tx.Commit(ctx); err != nil {
			log.Print("Commit message failed")
			http.Error(w, "Could not create message",
				http.StatusInternalServerError)
			return
		}

		// 10. RETURN THE STORED MESSAGE
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(message); err != nil {
			log.Printf("Write created message response: %v", err)
		}
	}
}
