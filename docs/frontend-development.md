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
