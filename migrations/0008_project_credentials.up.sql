-- Milestone 7 slice 2: per-project GitLab credentials.
-- Stores each project's GitLab access token encrypted at rest (envelope
-- encryption: a per-token data key wrapped by the master key from the secret
-- provider). Only ciphertext is persisted here; the plaintext token never
-- touches the database. One row per project; deleting a project removes its
-- credential. This migration is reversible; see the down file.

BEGIN;

CREATE TABLE project_credentials (
    project_id       uuid PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    token_ciphertext bytea NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT project_credentials_ciphertext_not_empty CHECK (octet_length(token_ciphertext) > 0)
);

COMMIT;
