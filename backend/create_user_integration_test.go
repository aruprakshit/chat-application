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
	"golang.org/x/crypto/bcrypt"
)

func TestCreateUserIntegration(t *testing.T) {
	// 1. REQUIRE AN EXPLICIT TEST DATABASE
	// Ordinary unit-test runs skip this test.
	// Never fall back to the application's DATABASE_URL.
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	// 2. ARRANGE A UNIQUE USER AND ROUTER
	// A random suffix prevents collisions between repeated test runs.
	username := "test_" + strings.ToLower(rand.Text())
	password := "test-only-password-123"
	body := `{"username":"` + username + `","password":"` + password + `"}`
	router := newRouter(pool)

	// Remove this test's row even if a later assertion fails.
	// Deferred functions run in reverse order, so this runs before Close.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 3*time.Second,
		)
		defer cleanupCancel()

		if _, err := pool.Exec(
			cleanupCtx,
			"DELETE FROM users WHERE username = $1",
			username,
		); err != nil {
			t.Errorf("clean up test user: %v", err)
		}
	}()

	// 3. ACT: CREATE THE USER THROUGH THE ROUTER
	request := httptest.NewRequest(
		http.MethodPost, "/users", strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	// 4. ASSERT: VERIFY THE SUCCESS RESPONSE
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s",
			recorder.Code, recorder.Body.String())
	}

	var created User
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}

	if created.ID <= 0 || created.Username != username {
		t.Fatalf("unexpected created user: %+v", created)
	}

	// Verify the response corresponds to an actual stored row.
	var storedUsername string
	err = pool.QueryRow(ctx,
		"SELECT username FROM users WHERE id = $1",
		created.ID,
	).Scan(&storedUsername)
	if err != nil {
		t.Fatalf("read stored user: %v", err)
	}
	if storedUsername != username {
		t.Fatalf("expected stored username %q, got %q",
			username, storedUsername)
	}
	// VERIFY THAT A USABLE HASH WAS STORED
	var storedHash string
	err = pool.QueryRow(ctx,
		"SELECT password_hash FROM users WHERE id = $1",
		created.ID,
	).Scan(&storedHash)
	if err != nil {
		t.Fatalf("read stored password hash: %v", err)
	}

	if storedHash == password {
		t.Fatal("database contains the plaintext password")
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(storedHash), []byte(password),
	); err != nil {
		t.Fatal("stored hash does not match the submitted password")
	}

	// VERIFY THAT THE RESPONSE CONTAINS ONLY PUBLIC FIELDS
	var responseFields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &responseFields); err != nil {
		t.Fatalf("decode response fields: %v", err)
	}
	for field := range responseFields {
		if field != "id" && field != "username" {
			t.Errorf("unexpected response field: %s", field)
		}
	}

	// 5. ACT: TRY THE SAME USERNAME AGAIN
	duplicateRequest := httptest.NewRequest(
		http.MethodPost, "/users", strings.NewReader(body),
	)
	duplicateRequest.Header.Set("Content-Type", "application/json")
	duplicateRecorder := httptest.NewRecorder()

	router.ServeHTTP(duplicateRecorder, duplicateRequest)

	// 6. ASSERT: REJECT THE DUPLICATE WITHOUT ADDING ANOTHER ROW
	if duplicateRecorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s",
			duplicateRecorder.Code, duplicateRecorder.Body.String())
	}

	var count int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM users WHERE username = $1",
		username,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly one stored user, got %d", count)
	}
}
