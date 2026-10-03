-- Each run adds 45 messages to the selected conversation.
\set ON_ERROR_STOP on

BEGIN;

INSERT INTO messages (conversation_id, sender_id, body)
SELECT
    cm.conversation_id,
    cm.user_id,
    '[pagination-seed] Message ' || lpad(n::text, 2, '0')
FROM conversation_members AS cm
CROSS JOIN generate_series(1, 45) AS seed(n)
WHERE cm.conversation_id = :conversation_id
  AND cm.user_id = :sender_id
ORDER BY n;

COMMIT;
