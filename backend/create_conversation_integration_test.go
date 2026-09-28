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

func TestCreateConversationIntegration(t *testing.T) {
	// 1. CONNECT ONLY TO THE EXPLICIT TEST DATABASE
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	// 2. TRACK FIXTURES AND REGISTER CLEANUP
	// Track only records created by this test.
	var userIDs []int64
	var tokens []string
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
			// Deleting a user also removes their sessions through CASCADE.
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM users WHERE id = $1", id,
			); err != nil {
				t.Errorf("clean up user: %v", err)
			}
		}
	}()

	// 3. ARRANGE THREE USERS WITH REAL SESSIONS
	// User 0: creator. User 1: participant. User 2: outsider.
	for i := 0; i < 3; i++ {
		username := "test_" + strings.ToLower(rand.Text())

		var userID int64
		err := pool.QueryRow(ctx, `
		       INSERT INTO users (username) VALUES ($1) RETURNING id`, username).Scan(&userID)
		if err != nil {
			t.Fatalf("create test user: %v", err)
		}
		userIDs = append(userIDs, userID)

		token, tokenHash := newSessionToken()
		tokens = append(tokens, token)

		_, err = pool.Exec(ctx, `
			INSERT INTO sessions (token_hash, user_id, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
		`, tokenHash, userID)
		if err != nil {
			t.Fatalf("create test session: %v", err)
		}
	}

	router := newRouter(pool)

	// 4. ACT: CREATE A CONVERSATION AS USER 0
	body := fmt.Sprintf(`{"participant_id":%d}`, userIDs[1])
	request := httptest.NewRequest(
		http.MethodPost,
		"/conversations",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{
		Name:  "session",
		Value: tokens[0],
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	// 5. ASSERT: SUCCESS RESPONSE AND EXACT MEMBERSHIPS
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s",
			recorder.Code, recorder.Body.String())
	}

	var created Conversation
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	conversationIDs = append(conversationIDs, created.ID)

	if created.ID <= 0 || created.Title != nil || created.CreatedAt.IsZero() {
		t.Fatalf("unexpected conversation response: %+v", created)
	}

	var totalMembers, expectedMembers int
	err = pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE user_id IN ($2, $3))
		FROM conversation_members
		WHERE conversation_id = $1
	`, created.ID, userIDs[0], userIDs[1]).Scan(
		&totalMembers,
		&expectedMembers,
	)
	if err != nil {
		t.Fatalf("inspect memberships: %v", err)
	}
	if totalMembers != 2 || expectedMembers != 2 {
		t.Fatalf("expected exactly the creator and participant; total=%d matched=%d",
			totalMembers, expectedMembers)
	}

	// 6. ASSERT: BOTH MEMBERS SEE IT; THE OUTSIDER DOES NOT
	for i, token := range tokens {
		listRequest := httptest.NewRequest(
			http.MethodGet, "/conversations", nil,
		)
		listRequest.AddCookie(&http.Cookie{
			Name:  "session",
			Value: token,
		})
		listRecorder := httptest.NewRecorder()

		router.ServeHTTP(listRecorder, listRequest)

		if listRecorder.Code != http.StatusOK {
			t.Fatalf("list conversations for user %d: got %d",
				i, listRecorder.Code)
		}

		var conversations []Conversation
		if err := json.Unmarshal(
			listRecorder.Body.Bytes(), &conversations,
		); err != nil {
			t.Fatalf("decode conversation list: %v", err)
		}

		if i < 2 {
			if len(conversations) != 1 || conversations[0].ID != created.ID {
				t.Errorf("member %d should see only the created conversation: %+v",
					i, conversations)
			}
		} else if len(conversations) != 0 {
			t.Errorf("outsider should see no conversations: %+v", conversations)
		}
	}

	// 7. ARRANGE A PARTICIPANT ID THAT NO LONGER EXISTS
	// Create and delete a fixture instead of guessing an unused ID.
	var missingID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO users (username) VALUES ($1) RETURNING id
	`, "test_"+strings.ToLower(rand.Text())).Scan(&missingID)
	if err != nil {
		t.Fatalf("create temporary participant: %v", err)
	}
	userIDs = append(userIDs, missingID)

	if _, err := pool.Exec(ctx,
		"DELETE FROM users WHERE id = $1", missingID,
	); err != nil {
		t.Fatalf("remove temporary participant: %v", err)
	}

	var countBefore int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM conversations",
	).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}

	// ACT: REQUEST CREATION WITH THE MISSING PARTICIPANT
	missingRequest := httptest.NewRequest(
		http.MethodPost,
		"/conversations",
		strings.NewReader(
			fmt.Sprintf(`{"participant_id":%d}`, missingID),
		),
	)
	missingRequest.Header.Set("Content-Type", "application/json")
	missingRequest.AddCookie(&http.Cookie{
		Name:  "session",
		Value: tokens[0],
	})
	missingRecorder := httptest.NewRecorder()

	router.ServeHTTP(missingRecorder, missingRequest)

	// ASSERT: RETURN 404 WITHOUT CREATING A CONVERSATION
	if missingRecorder.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", missingRecorder.Code)
	}

	var countAfter int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM conversations",
	).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != countBefore {
		t.Errorf("failed request created a conversation: before=%d after=%d",
			countBefore, countAfter)
	}

	// 8. VERIFY ROLLBACK AFTER CONVERSATION INSERTION
	t.Run("membership failure rolls back conversation", func(t *testing.T) {
		testCtx, testCancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer testCancel()

		// ARRANGE: Temporarily reject new memberships for our participant.
		// NOT VALID skips checking existing rows, but checks new writes.
		// The formatted value is an internally generated int64, not user text.
		_, err := pool.Exec(testCtx, fmt.Sprintf(`
		ALTER TABLE conversation_members
		ADD CONSTRAINT rtc027_reject_test_member
		CHECK (user_id <> %d) NOT VALID
	`, userIDs[1]))
		if err != nil {
			t.Fatalf("install failure constraint: %v", err)
		}

		// Remove the test-only constraint even if an assertion fails.
		// Subtest cleanup runs before the parent closes the database pool.
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer cleanupCancel()

			_, err := pool.Exec(cleanupCtx, `
			ALTER TABLE conversation_members
			DROP CONSTRAINT rtc027_reject_test_member
		`)
			if err != nil {
				t.Errorf("remove failure constraint: %v", err)
			}
		})

		// Record both table counts before making the request.
		var conversationsBefore, membershipsBefore int
		err = pool.QueryRow(testCtx, `
		SELECT
			(SELECT count(*) FROM conversations),
			(SELECT count(*) FROM conversation_members)
	`).Scan(&conversationsBefore, &membershipsBefore)
		if err != nil {
			t.Fatalf("read initial counts: %v", err)
		}

		// ACT: Send a valid request through authentication and the handler.
		request := httptest.NewRequest(
			http.MethodPost,
			"/conversations",
			strings.NewReader(
				fmt.Sprintf(`{"participant_id":%d}`, userIDs[1]),
			),
		)
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{
			Name:  "session",
			Value: tokens[0],
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		// ASSERT: The client gets a generic error, not a false success.
		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", recorder.Code)
		}
		if strings.TrimSpace(recorder.Body.String()) != "Could not create conversation" {
			t.Errorf("unexpected error response: %q", recorder.Body.String())
		}

		// ASSERT: Neither the conversation nor partial memberships survived.
		var conversationsAfter, membershipsAfter int
		err = pool.QueryRow(testCtx, `
		SELECT
			(SELECT count(*) FROM conversations),
			(SELECT count(*) FROM conversation_members)
	`).Scan(&conversationsAfter, &membershipsAfter)
		if err != nil {
			t.Fatalf("read final counts: %v", err)
		}

		if conversationsAfter != conversationsBefore {
			t.Errorf("conversation leaked: before=%d after=%d",
				conversationsBefore, conversationsAfter)
		}
		if membershipsAfter != membershipsBefore {
			t.Errorf("memberships leaked: before=%d after=%d",
				membershipsBefore, membershipsAfter)
		}
	})

	// 9. VERIFY THAT ROUTE PROTECTION BLOCKS REQUESTS BEFORE WRITING DATA
	t.Run("route protection creates no records", func(t *testing.T) {
		testCtx, testCancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer testCancel()

		// ARRANGE: A correctly formatted token that has no database session.
		unknownToken, _ := newSessionToken()

		// ARRANGE: A stored session that has already expired.
		// Set created_at in the past to satisfy our expiry constraint.
		expiredToken, expiredHash := newSessionToken()
		_, err := pool.Exec(testCtx, `
		INSERT INTO sessions (
			token_hash, user_id, created_at, expires_at
		)
		VALUES (
			$1,
			$2,
			CURRENT_TIMESTAMP - INTERVAL '2 hours',
			CURRENT_TIMESTAMP - INTERVAL '1 hour'
		)
	        `, expiredHash, userIDs[0])
		if err != nil {
			t.Fatalf("create expired session: %v", err)
		}

		// The parent's user cleanup also deletes this session via CASCADE.
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
				name:       "cross-origin request with valid session",
				token:      tokens[0],
				origin:     "https://untrusted.example",
				wantStatus: http.StatusForbidden,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE: Record the initial database state.
				var conversationsBefore, membershipsBefore int
				err := pool.QueryRow(testCtx, `
				SELECT
					(SELECT count(*) FROM conversations),
					(SELECT count(*) FROM conversation_members)
			`).Scan(&conversationsBefore, &membershipsBefore)
				if err != nil {
					t.Fatal(err)
				}

				request := httptest.NewRequest(
					http.MethodPost,
					"/conversations",
					strings.NewReader(
						fmt.Sprintf(`{"participant_id":%d}`, userIDs[1]),
					),
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

				// ACT: Exercise the real middleware chain.
				router.ServeHTTP(recorder, request)

				// ASSERT: Reject the request with the expected status.
				if recorder.Code != tt.wantStatus {
					t.Errorf("expected %d, got %d",
						tt.wantStatus, recorder.Code)
				}

				// ASSERT: Rejection must not leave database writes behind.
				var conversationsAfter, membershipsAfter int
				err = pool.QueryRow(testCtx, `
				SELECT
					(SELECT count(*) FROM conversations),
					(SELECT count(*) FROM conversation_members)
			`).Scan(&conversationsAfter, &membershipsAfter)
				if err != nil {
					t.Fatal(err)
				}

				if conversationsAfter != conversationsBefore ||
					membershipsAfter != membershipsBefore {
					t.Errorf(
						"rejected request changed rows: conversations %d→%d, memberships %d→%d",
						conversationsBefore, conversationsAfter,
						membershipsBefore, membershipsAfter,
					)
				}
			})
		}
	})
}
