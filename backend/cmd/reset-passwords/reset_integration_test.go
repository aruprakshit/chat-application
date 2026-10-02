package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestResetPasswordsTargetsOneUser(t *testing.T) {
	// ARRANGE: Require the explicitly configured test database.
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

	// ARRANGE: Track only this test's users for cleanup.
	var userIDs []int64

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, id := range userIDs {
			// Sessions are deleted through ON DELETE CASCADE.
			if _, err := pool.Exec(cleanupCtx,
				"DELETE FROM users WHERE id = $1", id,
			); err != nil {
				t.Errorf("clean up test user: %v", err)
			}
		}
	}()

	// ARRANGE: Give two users a known starting password.
	const oldPassword = "original-test-password"
	const newPassword = "replacement-test-password"

	oldHash, err := bcrypt.GenerateFromPassword(
		[]byte(oldPassword), bcrypt.DefaultCost,
	)
	if err != nil {
		t.Fatal("could not create fixture password hash")
	}

	usernames := []string{
		"reset_" + strings.ToLower(rand.Text()),
		"other_" + strings.ToLower(rand.Text()),
	}

	for _, username := range usernames {
		var id int64

		err := pool.QueryRow(ctx, `
			INSERT INTO users (username, password_hash)
			VALUES ($1, $2)
			RETURNING id
		`, username, string(oldHash)).Scan(&id)
		if err != nil {
			t.Fatalf("create fixture user: %v", err)
		}

		userIDs = append(userIDs, id)

		// A valid stored session hash is sufficient for this database test.
		// We are not exercising browser cookie authentication here.
		tokenHash := sha256.Sum256([]byte(rand.Text()))

		_, err = pool.Exec(ctx, `
			INSERT INTO sessions (token_hash, user_id, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
		`, tokenHash[:], id)
		if err != nil {
			t.Fatalf("create fixture session: %v", err)
		}
	}

	// ACT: Reset only the first user.
	count, err := resetPasswords(
		ctx,
		pool,
		resetOptions{Username: usernames[0]},
		newPassword,
	)
	if err != nil {
		t.Fatalf("reset password: %v", err)
	}

	// ASSERT: Exactly one user was reset.
	if count != 1 {
		t.Fatalf("expected 1 reset user, got %d", count)
	}

	// ASSERT: The selected user now has the new password.
	var targetHash string
	err = pool.QueryRow(ctx, `
		SELECT password_hash FROM users WHERE id = $1
	`, userIDs[0]).Scan(&targetHash)
	if err != nil {
		t.Fatalf("read target user: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(targetHash), []byte(newPassword),
	); err != nil {
		t.Error("target user's hash does not match the new password")
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(targetHash), []byte(oldPassword),
	); err == nil {
		t.Error("target user's old password still matches")
	}

	// ASSERT: The other user's stored hash is unchanged.
	var otherHash string
	err = pool.QueryRow(ctx, `
		SELECT password_hash FROM users WHERE id = $1
	`, userIDs[1]).Scan(&otherHash)
	if err != nil {
		t.Fatalf("read other user: %v", err)
	}

	if otherHash != string(oldHash) {
		t.Error("unselected user's password hash changed")
	}

	// ASSERT: Revoke only the selected user's sessions.
	for i, id := range userIDs {
		var sessionCount int

		err := pool.QueryRow(ctx, `
			SELECT count(*) FROM sessions WHERE user_id = $1
		`, id).Scan(&sessionCount)
		if err != nil {
			t.Fatalf("count sessions: %v", err)
		}

		wantCount := 1
		if i == 0 {
			wantCount = 0
		}

		if sessionCount != wantCount {
			t.Errorf("user %d: expected %d sessions, got %d",
				id, wantCount, sessionCount)
		}
	}
}

func TestResetPasswordsAllUsers(t *testing.T) {
	// ARRANGE: Use tables isolated from every other test.
	pool := isolatedTestPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer cancel()

	const newPassword = "bulk-replacement-password"

	var userIDs []int64

	for _, username := range []string{"bulk_alice", "bulk_charlie"} {
		var id int64

		// NULL represents a user whose password has not yet been set.
		err := pool.QueryRow(ctx, `
			INSERT INTO users (username)
			VALUES ($1)
			RETURNING id
		`, username).Scan(&id)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		userIDs = append(userIDs, id)

		tokenHash := sha256.Sum256([]byte(rand.Text()))

		_, err = pool.Exec(ctx, `
			INSERT INTO sessions (token_hash, user_id, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
		`, tokenHash[:], id)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	// ACT: Reset every user visible through the isolated pool.
	count, err := resetPasswords(
		ctx, pool, resetOptions{All: true}, newPassword,
	)
	if err != nil {
		t.Fatalf("bulk reset: %v", err)
	}

	// ASSERT: Both users received the new password.
	if count != 2 {
		t.Fatalf("expected 2 reset users, got %d", count)
	}

	hashes := make([]string, len(userIDs))

	for i, id := range userIDs {
		err := pool.QueryRow(ctx, `
			SELECT password_hash FROM users WHERE id = $1
		`, id).Scan(&hashes[i])
		if err != nil {
			t.Fatalf("read password hash: %v", err)
		}

		if err := bcrypt.CompareHashAndPassword(
			[]byte(hashes[i]), []byte(newPassword),
		); err != nil {
			t.Errorf("user %d does not have the new password", id)
		}
	}

	// ASSERT: Separate bcrypt calls generated independently salted hashes.
	if hashes[0] == hashes[1] {
		t.Error("expected different bcrypt hashes for the two users")
	}

	// ASSERT: All sessions in this isolated schema were revoked.
	var sessionCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM sessions
	`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	if sessionCount != 0 {
		t.Errorf("expected no sessions, got %d", sessionCount)
	}
}

func TestResetPasswordsRollsBackOnSessionDeletionFailure(t *testing.T) {
	// ARRANGE: Use an isolated schema with the real migrations.
	pool := isolatedTestPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer cancel()

	const username = "rollback_user"
	const oldPassword = "original-test-password"
	const newPassword = "replacement-test-password"

	oldHash, err := bcrypt.GenerateFromPassword(
		[]byte(oldPassword), bcrypt.DefaultCost,
	)
	if err != nil {
		t.Fatal("could not generate fixture hash")
	}

	var userID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id
	`, username, string(oldHash)).Scan(&userID)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	tokenHash := sha256.Sum256([]byte(rand.Text()))

	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
	`, tokenHash[:], userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// ARRANGE: Force DELETE to fail after the password UPDATE succeeds.
	// This trigger exists only in this test's isolated schema.
	_, err = pool.Exec(ctx, `
		CREATE FUNCTION reject_test_session_delete()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'forced session deletion failure';
		END;
		$$;

		CREATE TRIGGER reject_test_session_delete
		BEFORE DELETE ON sessions
		FOR EACH ROW
		EXECUTE FUNCTION reject_test_session_delete();
	`)
	if err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	// ACT: The reset must fail when it reaches session deletion.
	count, err := resetPasswords(
		ctx,
		pool,
		resetOptions{Username: username},
		newPassword,
	)

	// ASSERT: Report failure, not a successful reset.
	if err == nil {
		t.Fatal("expected session deletion to fail")
	}
	if err.Error() != "could not revoke sessions" {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected zero committed resets, got %d", count)
	}

	// ASSERT: Read committed state after the function's rollback.
	var storedHash string
	err = pool.QueryRow(ctx, `
		SELECT password_hash FROM users WHERE id = $1
	`, userID).Scan(&storedHash)
	if err != nil {
		t.Fatalf("read user after failure: %v", err)
	}

	if storedHash != string(oldHash) {
		t.Error("password changed despite transaction failure")
	}

	// ASSERT: The original session remains.
	var sessionCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM sessions
		WHERE user_id = $1 AND token_hash = $2
	`, userID, tokenHash[:]).Scan(&sessionCount)
	if err != nil {
		t.Fatalf("read session after failure: %v", err)
	}

	if sessionCount != 1 {
		t.Errorf("expected original session to remain, got %d", sessionCount)
	}
}
