package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func resetPasswords(
	ctx context.Context,
	pool *pgxpool.Pool,
	options resetOptions,
	password string,
) (int, error) {
	// 1. START THE TRANSACTION
	// Password updates and session deletion must commit together.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, errors.New("could not begin password reset transaction")
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 3*time.Second,
		)
		defer cancel()

		// After a successful commit, rollback is harmless.
		_ = tx.Rollback(cleanupCtx)
	}()

	// 2. SELECT AND LOCK THE TARGET USERS
	// Order locks consistently; prevent selected users from changing
	// or being deleted while this operation runs.
	query := "SELECT id FROM users ORDER BY id FOR UPDATE"
	var args []any

	if !options.All {
		query = "SELECT id FROM users WHERE username = $1 FOR UPDATE"
		args = []any{options.Username}
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return 0, errors.New("could not select users")
	}
	defer rows.Close()

	var userIDs []int64

	for rows.Next() {
		var id int64

		if err := rows.Scan(&id); err != nil {
			return 0, errors.New("could not read selected user")
		}
		userIDs = append(userIDs, id)
	}

	if err := rows.Err(); err != nil {
		return 0, errors.New("could not finish reading users")
	}

	// Release the result set before issuing another query on this connection.
	rows.Close()

	if len(userIDs) == 0 && !options.All {
		return 0, errors.New("specified username does not exist")
	}

	// 3. HASH AND UPDATE EACH PASSWORD SEPARATELY
	for _, userID := range userIDs {
		if err := ctx.Err(); err != nil {
			return 0, errors.New("password reset deadline exceeded or canceled")
		}

		hash, err := bcrypt.GenerateFromPassword(
			[]byte(password), bcrypt.DefaultCost,
		)
		if err != nil {
			return 0, errors.New("could not hash password")
		}

		_, err = tx.Exec(ctx, `
			UPDATE users
			SET password_hash = $1
			WHERE id = $2
		`, string(hash), userID)
		if err != nil {
			// Do not include database details that might expose hashes.
			return 0, errors.New("could not update password")
		}

		// 4. REVOKE THIS USER'S EXISTING SESSIONS
		_, err = tx.Exec(ctx, `
			DELETE FROM sessions WHERE user_id = $1
		`, userID)
		if err != nil {
			return 0, errors.New("could not revoke sessions")
		}
	}

	// 5. COMMIT BEFORE REPORTING SUCCESS
	if err := tx.Commit(ctx); err != nil {
		// A lost connection during commit can leave its outcome uncertain.
		return 0, errors.New(
			"could not confirm password reset commit; verify before retrying",
		)
	}

	return len(userIDs), nil
}

func executeReset(options resetOptions, config resetConfig) error {
	// 6. BOUND THE COMMAND'S DATABASE WORK
	ctx, cancel := context.WithTimeout(
		context.Background(), 30*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return errors.New("could not configure database connection")
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return errors.New("could not connect to database")
	}

	count, err := resetPasswords(ctx, pool, options, config.Password)
	if err != nil {
		return err
	}

	fmt.Printf(
		"Reset passwords for %d users and revoked their sessions.\n",
		count,
	)
	return nil
}
