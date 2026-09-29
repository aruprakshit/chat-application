package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateMessageIntegration(t *testing.T) {
	// ARRANGE: Connect only to the explicitly configured test database.
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	// ARRANGE: Track fixture IDs so cleanup also works after a failure.
	var userID, conversationID int64

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		// Delete dependent rows before their parent rows.
		statements := []struct {
			query string
			id    int64
		}{
			{"DELETE FROM messages WHERE conversation_id = $1", conversationID},
			{"DELETE FROM conversation_members WHERE conversation_id = $1", conversationID},
			{"DELETE FROM conversations WHERE id = $1", conversationID},
			{"DELETE FROM users WHERE id = $1", userID},
		}

		for _, statement := range statements {
			if _, err := pool.Exec(
				cleanupCtx, statement.query, statement.id,
			); err != nil {
				t.Errorf("clean up fixture: %v", err)
			}
		}
		// Deleting the user also deletes their sessions through ON DELETE CASCADE.
	}()

	// ARRANGE: Create a user and a conversation they belong to.
	username := "test_" + strings.ToLower(rand.Text())

	err = pool.QueryRow(ctx, `
		INSERT INTO users (username)
		VALUES ($1)
		RETURNING id
	`, username).Scan(&userID)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO conversations DEFAULT VALUES
		RETURNING id
	`).Scan(&conversationID)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id)
		VALUES ($1, $2)
	`, conversationID, userID)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	// ARRANGE: Store a real session so the request goes through authentication.
	token, tokenHash := newSessionToken()

	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
	`, tokenHash, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	router := newRouter(pool)

	t.Run("member stores message", func(t *testing.T) {
		// ARRANGE: Include Unicode and surrounding whitespace.
		// The handler must preserve the submitted text exactly.
		wantBody := "  Hello, 世界 👋\n"

		payload, err := json.Marshal(CreateMessageRequest{
			Body: wantBody,
		})
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}

		request := httptest.NewRequest(
			http.MethodPost,
			"/conversations/"+strconv.FormatInt(conversationID, 10)+"/messages",
			strings.NewReader(string(payload)),
		)
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		recorder := httptest.NewRecorder()

		// ACT: Exercise the complete route, including middleware.
		router.ServeHTTP(recorder, request)

		// ASSERT: The API returns the newly created message.
		if recorder.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d; body=%q",
				recorder.Code, recorder.Body.String())
		}

		var created Message
		if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if created.ID <= 0 ||
			created.ConversationID != conversationID ||
			created.SenderID != userID ||
			created.Body != wantBody ||
			created.CreatedAt.IsZero() {
			t.Fatalf("unexpected response: %+v", created)
		}

		// ASSERT: Read through the pool after the request has finished.
		// This verifies the message was committed, not merely returned.
		var stored Message
		err = pool.QueryRow(ctx, `
			SELECT id, conversation_id, sender_id, body, created_at
			FROM messages
			WHERE id = $1
		`, created.ID).Scan(
			&stored.ID,
			&stored.ConversationID,
			&stored.SenderID,
			&stored.Body,
			&stored.CreatedAt,
		)
		if err != nil {
			t.Fatalf("read stored message: %v", err)
		}

		if stored.ConversationID != conversationID ||
			stored.SenderID != userID ||
			stored.Body != wantBody ||
			!stored.CreatedAt.Equal(created.CreatedAt) {
			t.Errorf("stored message differs from response: %+v", stored)
		}
	})

	t.Run("nonmember cannot store message", func(t *testing.T) {
		// ARRANGE: Remove membership while keeping the user and session valid.
		_, err := pool.Exec(ctx, `
		DELETE FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2
	`, conversationID, userID)
		if err != nil {
			t.Fatalf("remove membership: %v", err)
		}

		// ARRANGE: Record the message count before the rejected request.
		var countBefore int
		err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM messages
		WHERE conversation_id = $1
	`, conversationID).Scan(&countBefore)
		if err != nil {
			t.Fatalf("count messages before request: %v", err)
		}

		request := httptest.NewRequest(
			http.MethodPost,
			"/conversations/"+strconv.FormatInt(conversationID, 10)+"/messages",
			strings.NewReader(`{"body":"This message must not be stored"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		recorder := httptest.NewRecorder()

		// ACT: Send valid input through the router using a valid session.
		router.ServeHTTP(recorder, request)

		// ASSERT: Hide the inaccessible conversation behind a 404 response.
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body=%q",
				recorder.Code, recorder.Body.String())
		}

		if strings.TrimSpace(recorder.Body.String()) != "Conversation not found" {
			t.Errorf("unexpected response: %q", recorder.Body.String())
		}

		// ASSERT: The rejected request must not add a message.
		var countAfter int
		err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM messages
		WHERE conversation_id = $1
	`, conversationID).Scan(&countAfter)
		if err != nil {
			t.Fatalf("count messages after request: %v", err)
		}

		if countAfter != countBefore {
			t.Errorf("rejected request changed message count: before=%d after=%d",
				countBefore, countAfter)
		}
	})

	t.Run("missing conversation cannot store message", func(t *testing.T) {
		// ARRANGE: Generate a real ID, then remove its conversation.
		var missingID int64
		err := pool.QueryRow(ctx, `
		INSERT INTO conversations DEFAULT VALUES
		RETURNING id
	`).Scan(&missingID)
		if err != nil {
			t.Fatalf("create temporary conversation: %v", err)
		}

		// Ensure cleanup is attempted even if the setup below fails.
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 3*time.Second,
			)
			defer cleanupCancel()

			if _, err := pool.Exec(cleanupCtx, `
			DELETE FROM conversations WHERE id = $1
		`, missingID); err != nil {
				t.Errorf("clean up temporary conversation: %v", err)
			}
		}()

		_, err = pool.Exec(ctx, `
		DELETE FROM conversations WHERE id = $1
	`, missingID)
		if err != nil {
			t.Fatalf("delete temporary conversation: %v", err)
		}

		request := httptest.NewRequest(
			http.MethodPost,
			"/conversations/"+strconv.FormatInt(missingID, 10)+"/messages",
			strings.NewReader(`{"body":"This must not be stored"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		recorder := httptest.NewRecorder()

		// ACT: Send a valid request with a valid session to the missing ID.
		router.ServeHTTP(recorder, request)

		// ASSERT: Match the response used for a nonmember.
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body=%q",
				recorder.Code, recorder.Body.String())
		}

		if strings.TrimSpace(recorder.Body.String()) != "Conversation not found" {
			t.Errorf("unexpected response: %q", recorder.Body.String())
		}

		// ASSERT: No message exists for the missing conversation.
		var count int
		err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM messages
		WHERE conversation_id = $1
	`, missingID).Scan(&count)
		if err != nil {
			t.Fatalf("count messages: %v", err)
		}

		if count != 0 {
			t.Errorf("expected no messages, got %d", count)
		}
	})

	t.Run("route protection creates no messages", func(t *testing.T) {
		// ARRANGE: Restore membership removed by the nonmember test.
		// A valid same-origin request would now be allowed to send.
		_, err := pool.Exec(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (conversation_id, user_id) DO NOTHING
	`, conversationID, userID)
		if err != nil {
			t.Fatalf("restore membership: %v", err)
		}

		unknownToken, _ := newSessionToken()
		expiredToken, expiredHash := newSessionToken()

		_, err = pool.Exec(ctx, `
		INSERT INTO sessions (
			token_hash, user_id, created_at, expires_at
		)
		VALUES (
			$1, $2,
			CURRENT_TIMESTAMP - INTERVAL '2 hours',
			CURRENT_TIMESTAMP - INTERVAL '1 hour'
		)
	`, expiredHash, userID)
		if err != nil {
			t.Fatalf("create expired session: %v", err)
		}
		// Existing fixture cleanup deletes this session with the user.

		tests := []struct {
			name       string
			token      string
			origin     string
			wantStatus int
		}{
			{
				name:       "missing cookie",
				wantStatus: http.StatusUnauthorized,
			},
			{
				name:       "malformed token",
				token:      "invalid-token",
				wantStatus: http.StatusUnauthorized,
			},
			{
				name:       "unknown session",
				token:      unknownToken,
				wantStatus: http.StatusUnauthorized,
			},
			{
				name:       "expired session",
				token:      expiredToken,
				wantStatus: http.StatusUnauthorized,
			},
			{
				name:       "cross-origin request",
				token:      token,
				origin:     "https://untrusted.example",
				wantStatus: http.StatusForbidden,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE: Record the count and prepare valid message input.
				var countBefore int
				err := pool.QueryRow(ctx, `
				SELECT count(*) FROM messages
				WHERE conversation_id = $1
			`, conversationID).Scan(&countBefore)
				if err != nil {
					t.Fatalf("count messages before request: %v", err)
				}

				request := httptest.NewRequest(
					http.MethodPost,
					"/conversations/"+strconv.FormatInt(conversationID, 10)+"/messages",
					strings.NewReader(`{"body":"Must not be stored"}`),
				)
				request.Header.Set("Content-Type", "application/json")

				if tt.token != "" {
					request.AddCookie(&http.Cookie{
						Name:  "session",
						Value: tt.token,
					})
				}
				if tt.origin != "" {
					request.Header.Set("Origin", tt.origin)
				}

				recorder := httptest.NewRecorder()

				// ACT: Exercise the complete middleware chain.
				router.ServeHTTP(recorder, request)

				// ASSERT: The appropriate middleware rejects the request.
				if recorder.Code != tt.wantStatus {
					t.Errorf("expected %d, got %d; body=%q",
						tt.wantStatus, recorder.Code, recorder.Body.String())
				}

				// ASSERT: Rejection leaves the stored messages unchanged.
				var countAfter int
				err = pool.QueryRow(ctx, `
				SELECT count(*) FROM messages
				WHERE conversation_id = $1
			`, conversationID).Scan(&countAfter)
				if err != nil {
					t.Fatalf("count messages after request: %v", err)
				}

				if countAfter != countBefore {
					t.Errorf("message count changed: before=%d after=%d",
						countBefore, countAfter)
				}
			})
		}
	})
}
