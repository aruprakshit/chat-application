# Database outage experiment

## Action
Stopped PostgreSQL while the Go backend remained running, requested both endpoints, then started PostgreSQL and requested users again.

## Expected behavior
- /health returns 200 while PostgreSQL is stopped.
- /users returns 500 while PostgreSQL is stopped.
- /users recovers after PostgreSQL restarts without restarting Go.

## Observed behavior
- After `docker compose stop db`, /users returned `500 Internal Server Error` with `Could not load users`.
- /health continued returning `200 OK` with `{"status":"ok","service":"chat-backend"}`.
- A second /users request also returned 500 while PostgreSQL was stopped.
- After `docker compose start db`, /users returned `200 OK` with `[]`, without restarting Go.

## Data clarification
The database volume had been deleted before this experiment, removing the previous records. The empty users response was not caused by stopping and starting PostgreSQL. The successful query shows the current users table exists but contained no rows.

## Lesson
A running HTTP server does not guarantee its dependencies are available. Database-backed requests failed during the outage and recovered after PostgreSQL restarted. Stopping a container preserves its named volume; deleting that volume removes its stored data.

## Follow-up: verify with a stored row
Completed and confirmed by the user:
- Inserted `alice`; GET /users returned HTTP 200 with `[{"id":1,"username":"alice"}]`.
- Stopped and started PostgreSQL, then repeated GET /users.
- The same user record survived the stop/start cycle.

This verifies row persistence across a database container stop/start. It does not test container removal or volume deletion.
