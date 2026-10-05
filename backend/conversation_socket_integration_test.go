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

	"github.com/coder/websocket"
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

	// ARRANGE: Start an actual HTTP server for the upgrade handshake.
	// ResponseRecorder cannot provide a real network connection.
	server := httptest.NewServer(router)
	defer server.Close()

	socketURL := "ws" + strings.TrimPrefix(server.URL, "http") +
		fmt.Sprintf("/ws/conversations/%d", conversationID)

	t.Run("member completes handshake", func(t *testing.T) {
		// ARRANGE
		dialCtx, dialCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer dialCancel()

		headers := make(http.Header)
		headers.Set("Origin", origin)
		headers.Set("Cookie", (&http.Cookie{
			Name:  "session",
			Value: tokens[0],
		}).String())

		// ACT: Send a proper WebSocket upgrade request.
		conn, response, err := websocket.Dial(
			dialCtx,
			socketURL,
			&websocket.DialOptions{HTTPHeader: headers},
		)
		if err != nil {
			t.Fatalf("connect as member: %v", err)
		}
		defer conn.CloseNow()

		// ASSERT: HTTP switched to the WebSocket protocol.
		if response.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("expected 101, got %d", response.StatusCode)
		}

		// ASSERT: Our temporary handler closes normally after accepting.
		// Reading processes the server's close frame.
		_, _, err = conn.Read(dialCtx)
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Fatalf("expected normal WebSocket closure, got %v", err)
		}
	})

	t.Run("nonmember cannot complete handshake", func(t *testing.T) {
		// ARRANGE: Same conversation, different authenticated user.
		dialCtx, dialCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer dialCancel()

		headers := make(http.Header)
		headers.Set("Origin", origin)
		headers.Set("Cookie", (&http.Cookie{
			Name:  "session",
			Value: tokens[1],
		}).String())

		// ACT
		conn, response, err := websocket.Dial(
			dialCtx,
			socketURL,
			&websocket.DialOptions{HTTPHeader: headers},
		)
		if conn != nil {
			defer conn.CloseNow()
		}

		// ASSERT: Authorization rejects the request before upgrading.
		if err == nil {
			t.Fatal("expected the nonmember connection to fail")
		}
		if response == nil {
			t.Fatal("expected an HTTP rejection response")
		}
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", response.StatusCode)
		}
	})

}
