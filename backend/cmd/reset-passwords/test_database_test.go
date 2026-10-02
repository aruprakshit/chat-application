package main

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isolatedTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	// 1. REQUIRE THE TEST DATABASE
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 30*time.Second,
	)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create admin pool: %v", err)
	}

	// Cleanups run in reverse registration order.
	t.Cleanup(admin.Close)

	// 2. CREATE A UNIQUE SCHEMA FOR THIS TEST
	schema := "reset_test_" + strings.ToLower(rand.Text())

	// SQL parameters cannot represent identifiers such as schema names.
	// Identifier.Sanitize quotes the identifier safely.
	quotedSchema := pgx.Identifier{schema}.Sanitize()

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		// Delete only the unique schema created by this helper.
		if _, err := admin.Exec(
			cleanupCtx,
			"DROP SCHEMA "+quotedSchema+" CASCADE",
		); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	// 3. POINT ALL CONNECTIONS IN THIS POOL AT THE ISOLATED SCHEMA
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}

	config.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("create isolated pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// 4. APPLY THE REAL SCHEMA DEFINITIONS
	// Tests run with this package directory as their working directory.
	migrationPaths, err := filepath.Glob(
		filepath.Join("..", "..", "migrations", "*.up.sql"),
	)
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	if len(migrationPaths) == 0 {
		t.Fatal("no migrations found")
	}

	// Glob returns sorted paths; numbered filenames define their order.
	// These existing migration files contain their own BEGIN/COMMIT.
	for _, path := range migrationPaths {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}

	return pool
}
