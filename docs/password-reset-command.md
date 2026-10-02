# Local password reset command

This command resets passwords for local development accounts.
It is not a public password-recovery endpoint.

Run all commands from the repository root in WSL.
Go and its dependencies run inside Docker.

## Start the backend

```bash
docker compose up -d backend
```

The command uses DATABASE_URL from the backend container.

## Enter the new password

For Bash:

```bash
read -r -s -p "New password: " RESET_PASSWORD
printf '\n'
export RESET_PASSWORD
```

For Zsh:

```zsh
read -r -s 'RESET_PASSWORD?New password: '
printf '\n'
export RESET_PASSWORD
```

Passwords must contain 12–72 bytes. Unicode characters can occupy
multiple bytes. The command preserves the supplied password exactly.

## Reset one account

```bash
docker compose exec -e RESET_PASSWORD backend \
  go run ./cmd/reset-passwords --username alice
```

Replace alice with the intended username.

Only that user's password is changed and their existing sessions revoked.
An unknown username causes the command to fail.

## Reset all local accounts

```bash
docker compose exec -e RESET_PASSWORD backend \
  go run ./cmd/reset-passwords --all
```

This assigns the supplied password to every selected user and revokes
their sessions. Each user receives an independently salted bcrypt hash.

An empty database succeeds with zero users reset.

## Clear the temporary environment variable

After running the command, whether it succeeds or fails:

```bash
unset RESET_PASSWORD
```

Do not place passwords in command arguments, committed files, or logs.

## Help

```bash
docker compose exec backend go run ./cmd/reset-passwords --help
```

Exactly one target is required: --username or --all.

## Transaction behavior

The command locks selected users, updates their passwords, and deletes
their sessions in one transaction.

Failures before commit roll back the changes. If commit confirmation
fails, its outcome may be uncertain; verify before retrying.

Users, conversations, memberships, and messages are preserved.

The command uses a 30-second operation deadline. An individual bcrypt
calculation cannot be interrupted by that deadline. Bulk resets are
intended for small local datasets.

## Verification

```bash
docker compose --profile test run --rm tests
docker compose exec backend go vet ./cmd/reset-passwords
```

Integration tests use TEST_DATABASE_URL. Bulk and failure tests use
isolated schemas so they do not reset other tests' users.

Coverage includes:
- Target and environment validation.
- Resetting one user while preserving another user's password and session.
- Bulk reset with independently salted hashes.
- Rollback when session deletion fails.
- Missing usernames and an empty bulk reset.

After a manual reset, refresh the browser and sign in with the new password.
An old session should no longer authenticate.
