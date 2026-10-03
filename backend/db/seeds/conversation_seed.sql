-- Each run creates three conversations with two members each.
\set ON_ERROR_STOP on

BEGIN;

-- 1. RESOLVE AND VALIDATE THE TWO USERS
-- Missing usernames fail NOT NULL; identical users fail CHECK.
CREATE TEMP TABLE seed_participants (
    current_user_id BIGINT NOT NULL,
    other_user_id BIGINT NOT NULL,
    CHECK (current_user_id <> other_user_id)
) ON COMMIT DROP;

INSERT INTO seed_participants (current_user_id, other_user_id)
VALUES (
    (SELECT id FROM users WHERE username = :'current_username'),
    (SELECT id FROM users WHERE username = :'other_username')
);

-- 2. CREATE CONVERSATIONS AND ADD BOTH MEMBERS
WITH created_conversations AS (
    INSERT INTO conversations (title)
    VALUES
        ('[conversation-seed] General'),
        ('[conversation-seed] Project planning'),
        ('[conversation-seed] Random')
    RETURNING id, title
),
created_memberships AS (
    INSERT INTO conversation_members (conversation_id, user_id)
    SELECT c.id, member.user_id
    FROM created_conversations AS c
    CROSS JOIN seed_participants AS p
    CROSS JOIN LATERAL (
        VALUES (p.current_user_id), (p.other_user_id)
    ) AS member(user_id)
    RETURNING conversation_id
)
-- 3. REPORT THE NEW CONVERSATIONS AND THEIR MEMBER COUNTS
SELECT
    c.id AS conversation_id,
    c.title,
    count(m.conversation_id) AS member_count
FROM created_conversations AS c
JOIN created_memberships AS m ON m.conversation_id = c.id
GROUP BY c.id, c.title
ORDER BY c.id;

COMMIT;
