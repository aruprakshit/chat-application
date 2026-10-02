package main

import (
	"errors"
	"flag"
	"io"
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
