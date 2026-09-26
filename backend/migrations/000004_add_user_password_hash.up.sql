BEGIN;

-- Store a password hash, never the original password.
-- Existing learning users have no password yet.
ALTER TABLE users
    ADD COLUMN password_hash TEXT;

COMMIT;
