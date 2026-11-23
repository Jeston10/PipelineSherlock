-- Reverts 0004: drop the occurrence uniqueness constraint.

BEGIN;

ALTER TABLE failure_occurrences
    DROP CONSTRAINT failure_occurrences_job_fingerprint_unique;

COMMIT;
