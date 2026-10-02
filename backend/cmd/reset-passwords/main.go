package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type resetOptions struct {
	Username string
	All      bool
}

func parseOptions(args []string, output io.Writer) (resetOptions, error) {
	// 1. DEFINE THIS COMMAND'S FLAGS
	// ContinueOnError returns parsing errors instead of exiting.
	flags := flag.NewFlagSet("reset-passwords", flag.ContinueOnError)
	flags.SetOutput(output)

	var options resetOptions

	flags.StringVar(
		&options.Username,
		"username",
		"",
		"Reset one local user's password",
	)
	flags.BoolVar(
		&options.All,
		"all",
		false,
		"Reset every local user's password",
	)

	if err := flags.Parse(args); err != nil {
		return resetOptions{}, err
	}

	// 2. REJECT UNEXPECTED POSITIONAL ARGUMENTS
	if flags.NArg() != 0 {
		return resetOptions{}, errors.New(
			"unexpected arguments; use --username <name> or --all",
		)
	}

	// 3. REQUIRE EXACTLY ONE TARGET
	// Visit distinguishes an omitted flag from --username="".
	usernameProvided := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "username" {
			usernameProvided = true
		}
	})

	if usernameProvided && strings.TrimSpace(options.Username) == "" {
		return resetOptions{}, errors.New("username cannot be blank")
	}

	if usernameProvided == options.All {
		return resetOptions{}, errors.New(
			"provide exactly one target: --username <name> or --all",
		)
	}

	return options, nil
}

func run(args []string) error {
	// 4. VALIDATE THE TARGET BEFORE ANY DATABASE WORK
	_, err := parseOptions(args, os.Stderr)
	if err != nil {
		return err
	}

	// Temporary checkpoint: valid flags do not yet execute a reset.
	return errors.New(
		"target accepted; password reset is not implemented yet",
	)
}

func main() {
	// 5. REPORT ERRORS AND SET THE PROCESS EXIT STATUS
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
