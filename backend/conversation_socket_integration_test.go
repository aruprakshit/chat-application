package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConversationSocketMembership(t *testing.T) {
	// 1. ARRANGE: CONNECT ONLY TO THE TEST DATABASE
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

	// 2. ARRANGE: REGISTER CLEANUP BEFORE CREATING FIXTURES
	var userIDs []int64
	var conversationID int64

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, query := range []string{
			"DELETE FROM conversation_members WHERE conversation_id = $1",
			"DELETE FROM conversations WHERE id = $1",
		} {
			if _, err := pool.Exec(
				cleanupCtx, query, conversationID,
			); err != nil {
				t.Errorf("clean up conversation: %v", err)
			}
		}

		for _, id := range userIDs {
			// Deleting the user also deletes their sessions.
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM users WHERE id = $1", id,
			); err != nil {
				t.Errorf("clean up user: %v", err)
			}
		}
	}()

	// 3. ARRANGE: CREATE TWO USERS WITH VALID SESSIONS
	tokens := make([]string, 2)

	for i := range tokens {
		username := "socket_" + strings.ToLower(rand.Text())

		var userID int64
		err := pool.QueryRow(ctx, `
			INSERT INTO users (username)
			VALUES ($1)
			RETURNING id
		`, username).Scan(&userID)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		userIDs = append(userIDs, userID)

		token, tokenHash := newSessionToken()
		tokens[i] = token

		_, err = pool.Exec(ctx, `
			INSERT INTO sessions (token_hash, user_id, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
		`, tokenHash, userID)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	// 4. ARRANGE: GIVE ONLY THE FIRST USER MEMBERSHIP
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
	`, conversationID, userIDs[0])
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	const origin = "http://localhost:3005"
	t.Setenv("WS_ALLOWED_ORIGIN", origin)
	router := newRouter(pool)

	tests := []struct {
		name       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "member reaches connection checkpoint",
			token:      tokens[0],
			wantStatus: http.StatusNotImplemented,
			wantBody:   "WebSocket connection is not implemented yet\n",
		},
		{
			name:       "nonmember cannot connect",
			token:      tokens[1],
			wantStatus: http.StatusNotFound,
			wantBody:   "Conversation not found\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Use an allowed origin and a real session cookie.
			request := httptest.NewRequest(
				http.MethodGet,
				fmt.Sprintf("/ws/conversations/%d", conversationID),
				nil,
			)
			request.Header.Set("Origin", origin)
			request.AddCookie(&http.Cookie{
				Name:  "session",
				Value: tt.token,
			})
			recorder := httptest.NewRecorder()

			// ACT: Exercise origin, authentication, and membership checks.
			router.ServeHTTP(recorder, request)

			// ASSERT
			if recorder.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body=%q",
					tt.wantStatus, recorder.Code, recorder.Body.String())
			}
			if recorder.Body.String() != tt.wantBody {
				t.Errorf("expected body %q, got %q",
					tt.wantBody, recorder.Body.String())
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Error("expected Cache-Control: no-store")
			}
		})
	}
}
