package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConversationsOnlyListsMemberships(t *testing.T) {
	// 1. CONNECT TO THE EXPLICIT TEST DATABASE
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	// 2. TRACK FIXTURES FOR CLEANUP
	// Fixtures are records created specifically for this test.
	// Delete dependent rows before the rows they reference.
	var userIDs []int64
	var conversationIDs []int64

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, id := range conversationIDs {
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM conversation_members WHERE conversation_id = $1",
				id,
			); err != nil {
				t.Errorf("clean up memberships: %v", err)
			}

			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM conversations WHERE id = $1", id,
			); err != nil {
				t.Errorf("clean up conversation: %v", err)
			}
		}

		for _, id := range userIDs {
			// Session rows are removed by ON DELETE CASCADE.
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM users WHERE id = $1", id,
			); err != nil {
				t.Errorf("clean up user: %v", err)
			}
		}
	}()

	// 3. ARRANGE TWO USERS WITH SEPARATE CONVERSATIONS
	// Passwords aren't needed: this test verifies session authentication
	// and membership filtering, not the login endpoint.
	for i := 0; i < 2; i++ {
		username := "test_" + strings.ToLower(rand.Text())

		var userID int64
		err := pool.QueryRow(ctx,
			"INSERT INTO users (username) VALUES ($1) RETURNING id",
			username,
		).Scan(&userID)
		if err != nil {
			t.Fatalf("create fixture user: %v", err)
		}
		userIDs = append(userIDs, userID)

		var conversationID int64
		err = pool.QueryRow(ctx,
			"INSERT INTO conversations (title) VALUES ($1) RETURNING id",
			"Authorization test",
		).Scan(&conversationID)
		if err != nil {
			t.Fatalf("create fixture conversation: %v", err)
		}
		conversationIDs = append(conversationIDs, conversationID)

		_, err = pool.Exec(ctx, `
			INSERT INTO conversation_members (conversation_id, user_id)
			VALUES ($1, $2)
		`, conversationID, userID)
		if err != nil {
			t.Fatalf("create fixture membership: %v", err)
		}
	}

	// 4. CREATE A REAL SESSION FOR THE FIRST USER
	token, tokenHash := newSessionToken()

	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
	`, tokenHash, userIDs[0])
	if err != nil {
		t.Fatalf("create fixture session: %v", err)
	}

	router := newRouter(pool)

	// 5. ACT: REQUEST CONVERSATIONS AS THE FIRST USER
	request := httptest.NewRequest(http.MethodGet, "/conversations", nil)
	request.AddCookie(&http.Cookie{
		Name:  "session",
		Value: token,
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	// 6. ASSERT: ONLY THE USER'S OWN CONVERSATION IS VISIBLE
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s",
			recorder.Code, recorder.Body.String())
	}

	var conversations []Conversation
	if err := json.Unmarshal(recorder.Body.Bytes(), &conversations); err != nil {
		t.Fatalf("decode conversations: %v", err)
	}

	if len(conversations) != 1 {
		t.Fatalf("expected exactly one conversation, got %d",
			len(conversations))
	}

	if conversations[0].ID != conversationIDs[0] {
		t.Errorf("expected conversation %d, got %d",
			conversationIDs[0], conversations[0].ID)
	}

	// 7. ASSERT: AN UNAUTHENTICATED REQUEST IS REJECTED
	anonymousRequest := httptest.NewRequest(
		http.MethodGet, "/conversations", nil,
	)
	anonymousRecorder := httptest.NewRecorder()

	router.ServeHTTP(anonymousRecorder, anonymousRequest)

	if anonymousRecorder.Code != http.StatusUnauthorized {
		t.Errorf("expected anonymous request to return 401, got %d",
			anonymousRecorder.Code)
	}
}
