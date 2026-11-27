-- Milestone 6: API authentication keys.
-- Stores only a SHA-256 hash of each API key, never the plaintext. The plaintext
-- is shown to the operator exactly once at creation, the same discipline used for
-- the per-project webhook secret. A key authenticates callers of the /api/v1
-- surface; the GitLab webhook endpoint keeps its own per-project token instead.
-- This migration is reversible; see 0006_api_keys.down.sql.

BEGIN;

CREATE TABLE api_keys (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name         text         NOT NULL,
    key_hash     text         NOT NULL,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    CONSTRAINT api_keys_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT api_keys_key_hash_unique UNIQUE (key_hash)
);

COMMIT;
