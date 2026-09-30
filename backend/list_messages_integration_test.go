package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListMessagesIntegration(t *testing.T) {
	// 1. ARRANGE: CONNECT TO THE TEST DATABASE
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
	var userID, conversationID int64

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

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
	}()

	// 3. ARRANGE: CREATE A MEMBER AND THEIR CONVERSATION
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

	// 4. ARRANGE: CREATE A REAL SESSION
	token, tokenHash := newSessionToken()

	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
	`, tokenHash, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// 5. ARRANGE: INSERT THREE MESSAGES IN A KNOWN ORDER
	// Capture generated IDs instead of assuming they start at 1.
	bodies := []string{"First message", "Second message", "Third message"}
	messageIDs := make([]int64, len(bodies))

	for i, body := range bodies {
		err = pool.QueryRow(ctx, `
			INSERT INTO messages (conversation_id, sender_id, body)
			VALUES ($1, $2, $3)
			RETURNING id
		`, conversationID, userID, body).Scan(&messageIDs[i])
		if err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	router := newRouter(pool)

	// Create a separate conversation that the existing test user can access.
	newMemberConversation := func(t *testing.T) int64 {
		t.Helper()

		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO conversations DEFAULT VALUES
			RETURNING id
		`).Scan(&id)
		if err != nil {
			t.Fatalf("create additional conversation: %v", err)
		}

		// Runs when the calling subtest finishes, including after Fatal.
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer cleanupCancel()

			for _, query := range []string{
				"DELETE FROM messages WHERE conversation_id = $1",
				"DELETE FROM conversation_members WHERE conversation_id = $1",
				"DELETE FROM conversations WHERE id = $1",
			} {
				if _, err := pool.Exec(cleanupCtx, query, id); err != nil {
					t.Errorf("clean up additional conversation: %v", err)
				}
			}
		})

		_, err = pool.Exec(ctx, `
			INSERT INTO conversation_members (conversation_id, user_id)
			VALUES ($1, $2)
		`, id, userID)
		if err != nil {
			t.Fatalf("create additional membership: %v", err)
		}

		return id
	}

	// 6. DEFINE A HELPER FOR AUTHENTICATED HISTORY REQUESTS
	fetchPage := func(
		t *testing.T,
		targetID int64,
		query string,
	) ListMessagesResponse {
		t.Helper()

		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf(
				"/conversations/%d/messages%s",
				targetID, query,
			),
			nil,
		)
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d; body=%q",
				recorder.Code, recorder.Body.String())
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Error("expected Cache-Control: no-store")
		}
		if recorder.Header().Get("Content-Type") != "application/json" {
			t.Error("expected application/json")
		}

		var page ListMessagesResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode page: %v", err)
		}

		return page
	}

	t.Run("reads consecutive pages", func(t *testing.T) {
		// ACT: Request two of the three messages.
		first := fetchPage(t, conversationID, "?limit=2")

		// ASSERT: Return newest first and preserve public message fields.
		if len(first.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(first.Messages))
		}

		for i, fixtureIndex := range []int{2, 1} {
			got := first.Messages[i]

			if got.ID != messageIDs[fixtureIndex] ||
				got.ConversationID != conversationID ||
				got.SenderID != userID ||
				got.Body != bodies[fixtureIndex] ||
				got.CreatedAt.IsZero() {
				t.Errorf("unexpected message at position %d: %+v", i, got)
			}
		}

		if first.NextCursor == nil {
			t.Fatal("expected a next cursor")
		}
		if *first.NextCursor != messageIDs[1] {
			t.Fatalf("expected cursor %d, got %d",
				messageIDs[1], *first.NextCursor)
		}

		// ACT: Follow the returned cursor.
		second := fetchPage(t, conversationID, fmt.Sprintf(
			"?limit=2&before_id=%d", *first.NextCursor,
		))

		// ASSERT: The remaining message appears exactly once.
		if len(second.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(second.Messages))
		}
		if second.Messages[0].ID != messageIDs[0] {
			t.Errorf("expected oldest message %d, got %d",
				messageIDs[0], second.Messages[0].ID)
		}
		if second.NextCursor != nil {
			t.Errorf("expected null cursor, got %d", *second.NextCursor)
		}
	})

	t.Run("exactly full final page has no cursor", func(t *testing.T) {
		// ACT: Request exactly the number of stored messages.
		page := fetchPage(t, conversationID, "?limit=3")

		// ASSERT: A full page alone does not prove another page exists.
		if len(page.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(page.Messages))
		}
		if page.NextCursor != nil {
			t.Error("expected null cursor when no extra row exists")
		}
	})

	t.Run("cursor beyond history returns empty array", func(t *testing.T) {
		// ACT: Request messages older than the oldest fixture message.
		page := fetchPage(t, conversationID, fmt.Sprintf(
			"?limit=2&before_id=%d", messageIDs[0],
		))

		// ASSERT: JSON [] decodes to a non-nil empty slice.
		if page.Messages == nil {
			t.Fatal("expected messages: [], got null or a missing field")
		}
		if len(page.Messages) != 0 {
			t.Errorf("expected no messages, got %d", len(page.Messages))
		}
		if page.NextCursor != nil {
			t.Error("expected null cursor for an empty page")
		}
	})

	t.Run("nonmember cannot read history", func(t *testing.T) {
		// ARRANGE: Keep the user and session valid, but remove membership.
		// The conversation still contains messages.
		_, err := pool.Exec(ctx, `
			DELETE FROM conversation_members
			WHERE conversation_id = $1 AND user_id = $2
		`, conversationID, userID)
		if err != nil {
			t.Fatalf("remove membership: %v", err)
		}

		// Restore membership afterward so later tests can reuse the fixture.
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 3*time.Second,
			)
			defer cleanupCancel()

			_, err := pool.Exec(cleanupCtx, `
				INSERT INTO conversation_members (conversation_id, user_id)
				VALUES ($1, $2)
				ON CONFLICT (conversation_id, user_id) DO NOTHING
			`, conversationID, userID)
			if err != nil {
				t.Errorf("restore membership: %v", err)
			}
		}()

		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/conversations/%d/messages", conversationID),
			nil,
		)
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		recorder := httptest.NewRecorder()

		// ACT: Request existing history using a valid session.
		router.ServeHTTP(recorder, request)

		// ASSERT: Reject access without disclosing message content.
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body=%q",
				recorder.Code, recorder.Body.String())
		}

		if got := recorder.Body.String(); got != "Conversation not found\n" {
			t.Errorf("unexpected response body: %q", got)
		}

		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Error("expected Cache-Control: no-store")
		}
	})

	t.Run("missing conversation returns not found", func(t *testing.T) {
		// ARRANGE: Generate an ID, then delete its conversation.
		// This avoids guessing whether an arbitrary ID exists.
		var missingID int64

		err := pool.QueryRow(ctx, `
			INSERT INTO conversations DEFAULT VALUES
			RETURNING id
		`).Scan(&missingID)
		if err != nil {
			t.Fatalf("create temporary conversation: %v", err)
		}

		// Attempt cleanup even if the next setup operation fails.
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
			http.MethodGet,
			fmt.Sprintf("/conversations/%d/messages", missingID),
			nil,
		)
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		recorder := httptest.NewRecorder()

		// ACT: Request a missing conversation using a valid session.
		router.ServeHTTP(recorder, request)

		// ASSERT: Match the response used for inaccessible conversations.
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body=%q",
				recorder.Code, recorder.Body.String())
		}

		if got := recorder.Body.String(); got != "Conversation not found\n" {
			t.Errorf("unexpected response body: %q", got)
		}

		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Error("expected Cache-Control: no-store")
		}
	})

	t.Run("empty conversation returns empty array", func(t *testing.T) {
		// ARRANGE: A conversation with membership but no messages.
		emptyID := newMemberConversation(t)

		// ACT
		page := fetchPage(t, emptyID, "")

		// ASSERT: Empty history is a successful response containing [].
		if page.Messages == nil {
			t.Fatal("expected messages: [], got null or a missing field")
		}
		if len(page.Messages) != 0 {
			t.Errorf("expected no messages, got %d", len(page.Messages))
		}
		if page.NextCursor != nil {
			t.Error("expected null cursor for empty history")
		}
	})

	t.Run("omitted limit defaults to twenty", func(t *testing.T) {
		// ARRANGE: Store enough messages to exceed the default page size.
		targetID := newMemberConversation(t)
		ids := make([]int64, 21)

		for i := range ids {
			err := pool.QueryRow(ctx, `
			INSERT INTO messages (conversation_id, sender_id, body)
			VALUES ($1, $2, $3)
			RETURNING id
		`, targetID, userID, fmt.Sprintf("Message %d", i+1)).Scan(&ids[i])
			if err != nil {
				t.Fatalf("create message: %v", err)
			}
		}

		// ACT: Omit limit entirely.
		page := fetchPage(t, targetID, "")

		// ASSERT: Return the newest 20 messages in descending ID order.
		if len(page.Messages) != 20 {
			t.Fatalf("expected 20 messages, got %d", len(page.Messages))
		}

		for i, message := range page.Messages {
			wantID := ids[20-i]
			if message.ID != wantID {
				t.Errorf("position %d: expected message %d, got %d",
					i, wantID, message.ID)
			}
		}

		// The oldest message remains for the next page.
		if page.NextCursor == nil {
			t.Fatal("expected a cursor for the remaining message")
		}
		if *page.NextCursor != ids[1] {
			t.Errorf("expected cursor %d, got %d",
				ids[1], *page.NextCursor)
		}
	})

	t.Run("history route requires authentication", func(t *testing.T) {
		// ARRANGE: Test both an absent cookie and an invalid token.
		tests := []struct {
			name  string
			token string
		}{
			{name: "missing cookie"},
			{name: "malformed token", token: "invalid-token"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE: Target an existing conversation containing messages.
				request := httptest.NewRequest(
					http.MethodGet,
					fmt.Sprintf("/conversations/%d/messages", conversationID),
					nil,
				)

				if tt.token != "" {
					request.AddCookie(&http.Cookie{
						Name:  "session",
						Value: tt.token,
					})
				}
				recorder := httptest.NewRecorder()

				// ACT: Exercise the router and its middleware.
				router.ServeHTTP(recorder, request)

				// ASSERT: Return an authentication error without message data.
				if recorder.Code != http.StatusUnauthorized {
					t.Fatalf("expected 401, got %d; body=%q",
						recorder.Code, recorder.Body.String())
				}

				if got := recorder.Body.String(); got != "Authentication required\n" {
					t.Errorf("unexpected response body: %q", got)
				}

				if recorder.Header().Get("Cache-Control") != "no-store" {
					t.Error("expected Cache-Control: no-store")
				}
			})
		}
	})
}
