-- Milestone 0 core schema for PipelineSherlock.
-- All identifiers are UUIDs (gen_random_uuid() is built into PostgreSQL 13+).
-- All timestamps are timestamptz and stored in UTC.
-- This migration is reversible; see 0001_init.down.sql.

BEGIN;

-- projects: GitLab projects registered for ingestion and analysis.
CREATE TABLE projects (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    gitlab_project_id  bigint       NOT NULL,
    name               text         NOT NULL,
    namespace          text         NOT NULL,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT projects_gitlab_project_id_positive CHECK (gitlab_project_id > 0),
    CONSTRAINT projects_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT projects_namespace_not_blank CHECK (length(btrim(namespace)) > 0),
    CONSTRAINT projects_gitlab_project_id_unique UNIQUE (gitlab_project_id)
);

-- webhook_events: durable inbox. The webhook endpoint persists here quickly and
-- a background worker processes asynchronously. delivery_key enables idempotent
-- de-duplication of redelivered events.
CREATE TABLE webhook_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    event_type    text         NOT NULL,
    delivery_key  text         NOT NULL,
    payload_json  jsonb        NOT NULL,
    received_at   timestamptz  NOT NULL DEFAULT now(),
    processed_at  timestamptz,
    status        text         NOT NULL DEFAULT 'received',
    error_code    text,
    CONSTRAINT webhook_events_status_valid
        CHECK (status IN ('received', 'processing', 'processed', 'failed')),
    CONSTRAINT webhook_events_delivery_key_unique UNIQUE (project_id, delivery_key)
);
CREATE INDEX webhook_events_status_idx ON webhook_events (status, received_at);

-- pipelines: GitLab pipeline runs.
CREATE TABLE pipelines (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    gitlab_pipeline_id bigint       NOT NULL,
    ref                text         NOT NULL,
    sha                text         NOT NULL,
    status             text         NOT NULL,
    started_at         timestamptz,
    finished_at        timestamptz,
    duration_seconds   integer,
    CONSTRAINT pipelines_gitlab_pipeline_unique UNIQUE (project_id, gitlab_pipeline_id)
);
CREATE INDEX pipelines_project_ref_idx ON pipelines (project_id, ref);

-- jobs: individual jobs within a pipeline.
CREATE TABLE jobs (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id      uuid         NOT NULL REFERENCES pipelines (id) ON DELETE CASCADE,
    gitlab_job_id    bigint       NOT NULL,
    name             text         NOT NULL,
    stage            text         NOT NULL,
    status           text         NOT NULL,
    started_at       timestamptz,
    finished_at      timestamptz,
    duration_seconds integer,
    CONSTRAINT jobs_gitlab_job_unique UNIQUE (pipeline_id, gitlab_job_id)
);
CREATE INDEX jobs_pipeline_idx ON jobs (pipeline_id);

-- failure_fingerprints: stable identity for a recurring failure signature.
CREATE TABLE failure_fingerprints (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id           uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    fingerprint          text         NOT NULL,
    category             text         NOT NULL,
    normalized_signature text         NOT NULL,
    algorithm_version    integer      NOT NULL DEFAULT 1,
    first_seen_at        timestamptz  NOT NULL DEFAULT now(),
    last_seen_at         timestamptz  NOT NULL DEFAULT now(),
    occurrence_count     bigint       NOT NULL DEFAULT 0,
    CONSTRAINT failure_fingerprints_unique UNIQUE (project_id, fingerprint, algorithm_version)
);
CREATE INDEX failure_fingerprints_category_idx ON failure_fingerprints (project_id, category);

-- failure_occurrences: a single observed instance of a fingerprinted failure.
CREATE TABLE failure_occurrences (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id         uuid         NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    fingerprint_id uuid         NOT NULL REFERENCES failure_fingerprints (id) ON DELETE CASCADE,
    category       text         NOT NULL,
    evidence_excerpt text       NOT NULL,
    evidence_hash  text         NOT NULL,
    observed_at    timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX failure_occurrences_fingerprint_idx ON failure_occurrences (fingerprint_id, observed_at);

-- comments: merge-request comments posted back to GitLab (opt-in feature).
CREATE TABLE comments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fingerprint_id   uuid         NOT NULL REFERENCES failure_fingerprints (id) ON DELETE CASCADE,
    merge_request_iid bigint      NOT NULL,
    gitlab_note_id   bigint,
    posted_at        timestamptz,
    status           text         NOT NULL DEFAULT 'pending',
    CONSTRAINT comments_status_valid
        CHECK (status IN ('pending', 'posted', 'failed'))
);

COMMIT;
