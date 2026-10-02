# Frontend development

Run commands from the repository root in a WSL terminal.
Node.js and npm run through Docker; no host installation is required.

## First-time setup

Set the container user to match the WSL user:

```bash
export LOCAL_UID="$(id -u)"
export LOCAL_GID="$(id -g)"
```

Install the dependencies recorded in package-lock.json:

```bash
docker compose run --rm frontend npm ci
```

Start the application:

```bash
docker compose up -d backend frontend
```

Open http://localhost:3000.

Source changes are visible through the bind mount and development reload.
Dependencies are stored in frontend/node_modules and excluded from Git.

## Networking

The browser sends API requests to http://localhost:3000/api/*.

Next.js forwards them to BACKEND_URL, configured in Compose as
http://backend:9090. The /api prefix is removed before forwarding.

The service name backend resolves inside the Compose network.
Host access to Go uses http://localhost:8080.

After changing Compose environment variables, run docker compose up -d
to recreate affected containers. Restart alone does not apply those changes.

## Authentication

Use an existing account created through the Go API.

Login sets an HttpOnly session cookie. The frontend calls /api/me
to restore the authenticated user after a refresh.

Logout revokes the server session and clears the cookie.
The frontend does not store authentication tokens in local storage.

COOKIE_SECURE=false is used for local HTTP development.
HTTPS deployments must use secure cookies.

## Development checks

```bash
docker compose exec frontend npm run format:check
docker compose exec frontend npm run lint
docker compose exec frontend npx tsc --noEmit
docker compose --profile test run --rm tests
```

To apply formatting:

```bash
docker compose exec frontend npm run format
```

## Logs and troubleshooting

```bash
docker compose logs -f frontend backend
```

Check API forwarding:

```bash
curl -i http://localhost:3000/api/health
```

For a backend-outage experiment:

```bash
docker compose stop backend
docker compose start backend
```

Expected behavior:
- Failed login displays an error and re-enables the form.
- Failed logout displays an error and keeps the signed-in view.
- Failed session restoration displays a retry screen.
- Restoring the backend allows requests to succeed again.

## Stop development services

```bash
docker compose stop frontend backend
```

This stops the application services while leaving PostgreSQL running.

## Conversation isolation and session revocation verification

Verification record: 2026-10-02.

### Automated checks

The following checks completed successfully through Docker:

- `docker compose exec frontend npm run format:check`
- `docker compose exec frontend npm run lint`
- `docker compose exec frontend npx tsc --noEmit`
- `docker compose --profile test run --rm tests`

The Go suite includes conversation membership filtering and authentication
checks. These do not replace the browser checks below.

### Account isolation (passed)

Inspect the expected memberships:

```bash
docker compose exec db psql -U chat -d chat -c \
  "SELECT u.username, cm.conversation_id
   FROM conversation_members cm
   JOIN users u ON u.id = cm.user_id
   ORDER BY u.username, cm.conversation_id;"
```

Sign in as two accounts with different memberships, using a normal browser
window and a private window so their cookies are separate. Compare the lists
with the query results. Each account should see only its memberships; shared
conversations should appear for both members.

Accounts tested (user-reported): `alice` and `charlie`.

Result (user-confirmed on 2026-10-02): both accounts saw only their own conversation memberships, matching the expected results.
Do not record passwords or session tokens.

### Session revocation (passed)

Result (user-confirmed on 2026-10-02): retrying after session revocation returned the UI to the login form with the expiry message. Signing in again succeeded and restored the conversation list.

1. While signed in, block `*api/conversations*` in browser developer tools
   and refresh. Allow `/api/me` to succeed. The conversation panel should
   display its request error and retry button.
2. Disable request blocking without refreshing or clicking retry yet.
3. Revoke sessions for the chosen local test account using the SQL below.
4. Click the conversation panel's **Try again** button.
5. Confirm `/api/conversations` returns `401` and the login form displays
   **Your session has expired. Please sign in again.**
6. Sign in again and confirm the conversation list returns.

Replace `charlie` with the intended local test username. This revokes all
sessions for that account while preserving its data:

```bash
docker compose exec db psql -U chat -d chat -c \
  "DELETE FROM sessions
   WHERE user_id = (
     SELECT id FROM users WHERE username = 'charlie'
   );"
```

This simulates revocation, not the passage of the session expiry time. Both
revoked and expired sessions are rejected with `401` by authentication.

A full page refresh instead exercises `/api/me`; it does not verify the
conversation panel's session-expiry callback. A database outage may return
`503` during authentication and must show a session-check error rather than
claim that the session expired. The session-check error screen was observed
in the user's browser. Account isolation and the panel's revocation flow
were subsequently confirmed by the user as working as expected.
