BEGIN;

-- 1. STORE SERVER-SIDE SESSIONS
-- Store a hash of the random token, not the token sent to the browser.
CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,

    -- SHA-256 produces exactly 32 bytes.
    CONSTRAINT sessions_token_hash_length
        CHECK (octet_length(token_hash) = 32),

    CONSTRAINT sessions_expiry_valid
        CHECK (expires_at > created_at)
);

-- 2. SUPPORT REVOKING ALL SESSIONS FOR A USER
CREATE INDEX sessions_user_id_idx
    ON sessions (user_id);

-- 3. SUPPORT CLEANING UP EXPIRED SESSIONS
CREATE INDEX sessions_expires_at_idx
    ON sessions (expires_at);

COMMIT;
