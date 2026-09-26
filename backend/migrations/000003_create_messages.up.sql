BEGIN;

-- 1. STORE EACH MESSAGE
-- Foreign keys ensure the conversation and sender exist.
CREATE TABLE messages (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id),
    sender_id BIGINT NOT NULL REFERENCES users(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Reject empty/space-only messages and limit stored text length.
    CONSTRAINT messages_body_valid CHECK (
        char_length(btrim(body)) > 0
        AND char_length(body) <= 4000
    )
);

-- 2. SUPPORT FETCHING HISTORY FOR ONE CONVERSATION
-- First locate the conversation, then read its messages by ID.
CREATE INDEX messages_conversation_id_id_idx
    ON messages (conversation_id, id);

COMMIT;
