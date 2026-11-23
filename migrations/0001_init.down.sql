-- Reverse of 0001_init.up.sql. Drops tables in dependency order.

BEGIN;

DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS failure_occurrences;
DROP TABLE IF EXISTS failure_fingerprints;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS pipelines;
DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS projects;

COMMIT;
