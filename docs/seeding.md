# Local development seed data

Run these commands from the repository root in a Bash/WSL terminal.
Use only against the local development database. The scripts write directly
through SQL and bypass the Go HTTP handlers.

Start the application database and apply migrations before seeding:

```bash
docker compose up -d backend
```

The seed scripts live in `backend/db/seeds/`. Each script uses a transaction
and stops on SQL errors. Each run adds new data; rerunning does not replace
or deduplicate earlier seed data.

## Find users and memberships

```bash
docker compose exec db psql -U chat -d chat -c \
  "SELECT id, username FROM users ORDER BY id;"

docker compose exec db psql -U chat -d chat -c \
  "SELECT cm.conversation_id, u.id AS user_id, u.username
   FROM conversation_members cm
   JOIN users u ON u.id = cm.user_id
   ORDER BY cm.conversation_id, u.id;"
```

SQL cannot discover which user is signed in through your browser. Supply
that account's username when creating conversations, and a member's user ID
when inserting messages. These scripts do not create user accounts.

## Seed conversations

Replace `charlie` and `alice` with two different existing usernames:

```bash
docker compose exec -T db psql \
  -U chat -d chat \
  -v ON_ERROR_STOP=1 \
  -v current_username=charlie \
  -v other_username=alice \
  < backend/db/seeds/conversation_seed.sql
```

Each run creates three conversations:

- `[conversation-seed] General`
- `[conversation-seed] Project planning`
- `[conversation-seed] Random`

Both users become members of all three conversations. The output reports
the new conversation IDs and a `member_count` of `2` for each.

A missing username or two identical usernames causes an error and prevents
the transaction from committing. The temporary participant table is removed
when the transaction commits.

Refresh the browser's conversation list to see the new conversations.
They start with no messages, which lets you verify the empty-history view.
Use a returned conversation ID in the pagination command below.

## Seed pagination messages

Replace `conversation_id` and `sender_id` with a conversation ID and the ID
of a user who belongs to it. The values below are examples:

```bash
docker compose exec -T db psql \
  -U chat -d chat \
  -v ON_ERROR_STOP=1 \
  -v conversation_id=1 \
  -v sender_id=6 \
  < backend/db/seeds/pagination_seed.sql
```

Each run adds 45 messages numbered `[pagination-seed] Message 01` through
`[pagination-seed] Message 45`. Expected: `INSERT 0 45`.
`INSERT 0 0` means no membership matched the supplied conversation and user.

The shell reads the SQL file from your checkout and passes it to `psql`
inside Docker. `-T` disables terminal allocation for this redirected input;
the file does not need to be mounted into the database container.

## Verify pagination in the browser

Refresh or reselect the conversation after seeding.
For an otherwise empty conversation with a page size of 20:

| Action | Messages in the API response | Total displayed in the UI |
| --- | --- | --- |
| Open conversation | 20 (seed numbers 26-45) | 20 |
| Load older once | 20 (seed numbers 06-25) | 40 |
| Load older again | 5 (seed numbers 01-05) | 45 |

The API returns each page newest first. The UI reverses each page for
reading order and prepends older messages while retaining those already
loaded. The final response has `next_cursor: null`, so the button disappears.

Existing messages and repeated seed runs increase the total history and may
produce additional pages. Seed numbers are labels in the body, not message
IDs. Database IDs may contain gaps.

Check the actual count, replacing `1` with your conversation ID:

```bash
docker compose exec db psql -U chat -d chat -c \
  "SELECT count(*) AS total_messages
   FROM messages WHERE conversation_id = 1;"
```

## Optional pagination cleanup

The following deletes all messages with the pagination seed prefix in the
chosen conversation, including messages from repeated seed runs. Replace
`1` with the conversation ID you seeded:

```bash
docker compose exec db psql -U chat -d chat -c \
  "DELETE FROM messages
   WHERE conversation_id = 1
     AND body LIKE '[pagination-seed] Message %';"
```

This keeps the conversation and its memberships. Refresh the browser after
cleanup to reload the stored history.
