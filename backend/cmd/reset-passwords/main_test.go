package main

import (
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

func TestParseOptionsAcceptsOneTarget(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want resetOptions
	}{
		{
			name: "one user",
			args: []string{"--username", "alice"},
			want: resetOptions{Username: "alice"},
		},
		{
			name: "all users",
			args: []string{"--all"},
			want: resetOptions{All: true},
		},
		{
			name: "equals syntax",
			args: []string{"--username=charlie"},
			want: resetOptions{Username: "charlie"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Arguments and expected options come from the table.

			// ACT: Discard help/error output during this test.
			got, err := parseOptions(tt.args, io.Discard)

			// ASSERT: Parsing succeeds and selects the intended target.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestParseOptionsRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "no target",
			args: nil,
		},
		{
			name: "both targets",
			args: []string{"--username", "alice", "--all"},
		},
		{
			name: "empty username",
			args: []string{"--username="},
		},
		{
			name: "blank username",
			args: []string{"--username", "   "},
		},
		{
			name: "username value missing",
			args: []string{"--username"},
		},
		{
			name: "unexpected positional argument",
			args: []string{"--all", "alice"},
		},
		{
			name: "unknown flag",
			args: []string{"--everyone"},
		},
		{
			name: "disabled all without username",
			args: []string{"--all=false"},
		},
		{
			name: "invalid boolean",
			args: []string{"--all=maybe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Supply an invalid argument combination.

			// ACT
			_, err := parseOptions(tt.args, io.Discard)

			// ASSERT: Invalid input must not be accepted.
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseOptionsHelp(t *testing.T) {
	// ARRANGE
	args := []string{"--help"}

	// ACT
	_, err := parseOptions(args, io.Discard)

	// ASSERT: main uses this special result to exit successfully.
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestLoadConfig(t *testing.T) {
	// This URL is a test value only; these tests never connect.
	const databaseURL = "postgres://test:test@db:5432/test"

	tests := []struct {
		name        string
		databaseURL string
		password    string
		wantError   bool
	}{
		{
			name:      "missing database URL",
			password:  "password1234",
			wantError: true,
		},
		{
			name:        "blank database URL",
			databaseURL: "   ",
			password:    "password1234",
			wantError:   true,
		},
		{
			name:        "missing password",
			databaseURL: databaseURL,
			wantError:   true,
		},
		{
			name:        "eleven bytes rejected",
			databaseURL: databaseURL,
			password:    "password123",
			wantError:   true,
		},
		{
			name:        "twelve bytes accepted",
			databaseURL: databaseURL,
			password:    "password1234",
		},
		{
			name:        "seventy-two bytes accepted",
			databaseURL: databaseURL,
			password:    strings.Repeat("a", 72),
		},
		{
			name:        "seventy-three bytes rejected",
			databaseURL: databaseURL,
			password:    strings.Repeat("a", 73),
			wantError:   true,
		},
		{
			name:        "Unicode measured in bytes",
			databaseURL: databaseURL,
			password:    strings.Repeat("界", 25), // 75 UTF-8 bytes.
			wantError:   true,
		},
		{
			name:        "password whitespace preserved",
			databaseURL: databaseURL,
			password:    " password1234 ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE: Supply controlled environment values.
			getenv := func(key string) string {
				switch key {
				case "DATABASE_URL":
					return tt.databaseURL
				case "RESET_PASSWORD":
					return tt.password
				default:
					return ""
				}
			}

			// ACT
			got, err := loadConfig(getenv)

			// ASSERT: Reject invalid configuration.
			if tt.wantError {
				if err == nil {
					t.Fatal("expected a configuration error")
				}
				return
			}

			// ASSERT: Preserve valid values exactly.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.DatabaseURL != tt.databaseURL {
				t.Error("database URL was unexpectedly changed")
			}
			if got.Password != tt.password {
				t.Error("password was unexpectedly changed")
			}
		})
	}
}
