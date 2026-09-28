package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateConversationRequest struct {
	ParticipantID int64 `json:"participant_id"`
}

func createConversationHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. READ THE AUTHENTICATED CREATOR
		// Trust the identity established by middleware, not the request body.
		user, ok := r.Context().Value(authContextKey{}).(User)
		if !ok {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// 2. REQUIRE JSON INPUT
		contentType, _, err := mime.ParseMediaType(
			r.Header.Get("Content-Type"),
		)
		if err != nil || contentType != "application/json" {
			http.Error(w, "Content-Type must be application/json",
				http.StatusUnsupportedMediaType)
			return
		}

		// 3. DECODE ONE SMALL JSON OBJECT
		// Reject oversized input, unknown fields, and extra JSON values.
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		defer r.Body.Close()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input CreateConversationRequest
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Expected a JSON object with participant_id",
				http.StatusBadRequest)
			return
		}

		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "Body must contain exactly one JSON value",
				http.StatusBadRequest)
			return
		}

		// 4. VALIDATE THE PARTICIPANT ID
		// Missing and null IDs also become zero in this request struct.
		if input.ParticipantID <= 0 {
			http.Error(w, "participant_id must be a positive integer",
				http.StatusBadRequest)
			return
		}

		if input.ParticipantID == user.ID {
			http.Error(w, "Choose another user as the participant",
				http.StatusBadRequest)
			return
		}

		// 5. START A DATABASE TRANSACTION
		// All transaction operations share a three-second deadline.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Printf("Begin conversation transaction: %v", err)
			http.Error(w, "Could not create conversation",
				http.StatusInternalServerError)
			return
		}

		// Roll back on any early return.
		// Use a fresh context so cleanup can run if the request timed out.
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 3*time.Second,
			)
			defer cleanupCancel()

			err := tx.Rollback(cleanupCtx)
			if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				log.Printf("Roll back conversation: %v", err)
			}
		}()

		// 6. CHECK THAT THE PARTICIPANT EXISTS
		// The row lock prevents this user from being deleted before we finish.
		var participantID int64

		err = tx.QueryRow(ctx, `
			SELECT id FROM users WHERE id = $1 FOR KEY SHARE
		`, input.ParticipantID).Scan(&participantID)

		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Participant not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("Look up participant: %v", err)
			http.Error(w, "Could not create conversation",
				http.StatusInternalServerError)
			return
		}

		// 7. CREATE THE CONVERSATION
		// DEFAULT VALUES generates the ID and timestamps; title remains NULL.
		var conversation Conversation

		err = tx.QueryRow(ctx, `
			INSERT INTO conversations DEFAULT VALUES
			RETURNING id, title, created_at
		`).Scan(
			&conversation.ID,
			&conversation.Title,
			&conversation.CreatedAt,
		)
		if err != nil {
			log.Printf("Insert conversation: %v", err)
			http.Error(w, "Could not create conversation",
				http.StatusInternalServerError)
			return
		}

		// 8. ADD BOTH MEMBERS WITHIN THE SAME TRANSACTION
		// The creator's ID comes from authentication, never from the body.
		_, err = tx.Exec(ctx, `
			INSERT INTO conversation_members (conversation_id, user_id)
			VALUES ($1, $2), ($1, $3)
		`, conversation.ID, user.ID, participantID)
		if err != nil {
			log.Printf("Insert conversation members: %v", err)
			http.Error(w, "Could not create conversation",
				http.StatusInternalServerError)
			return
		}

		// 9. COMMIT BEFORE SENDING SUCCESS
		// INSERT returning an ID does not mean the transaction has committed.
		if err := tx.Commit(ctx); err != nil {
			log.Printf("Commit conversation: %v", err)
			http.Error(w, "Could not create conversation",
				http.StatusInternalServerError)
			return
		}

		// 10. WRITE THE COMMITTED CONVERSATION AS JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(conversation); err != nil {
			log.Printf("Write created conversation: %v", err)
		}
	}
}
