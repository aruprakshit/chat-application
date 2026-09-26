BEGIN;

-- 1. STORE EACH CONVERSATION
-- A missing title is allowed, which is useful for direct conversations.
CREATE TABLE conversations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 2. CONNECT USERS TO CONVERSATIONS
-- Each row represents one user's membership in one conversation.
CREATE TABLE conversation_members (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id),
    user_id BIGINT NOT NULL REFERENCES users(id),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT conversation_members_pair_unique
        UNIQUE (conversation_id, user_id)
);

-- 3. SUPPORT LOOKING UP A USER'S CONVERSATIONS
-- The primary key already supports lookups beginning with conversation_id.
-- This index supports lookups beginning with user_id.
CREATE INDEX conversation_members_user_id_idx
    ON conversation_members (user_id, conversation_id);

COMMIT;
