-- Milestone 2 slice 2: make occurrence recording idempotent.
-- A worker may reprocess the same job, so (job_id, fingerprint_id) must be
-- unique; the FingerprintRepo relies on ON CONFLICT DO NOTHING and only bumps
-- occurrence_count when a genuinely new occurrence row is inserted.

BEGIN;

ALTER TABLE failure_occurrences
    ADD CONSTRAINT failure_occurrences_job_fingerprint_unique UNIQUE (job_id, fingerprint_id);

COMMIT;
