-- Deterministic demo dataset for local development and the demo walkthrough.
-- Safe to run repeatedly: it upserts a single demo project and its pipelines.
-- All data here is synthetic.

BEGIN;

-- One demo project.
INSERT INTO projects (gitlab_project_id, name, namespace)
VALUES (1001, 'checkout-service', 'demo/platform')
ON CONFLICT (gitlab_project_id) DO UPDATE
    SET name = EXCLUDED.name, namespace = EXCLUDED.namespace;

-- Two pipelines on the demo project: one passed, one failed.
INSERT INTO pipelines (project_id, gitlab_pipeline_id, ref, sha, status, started_at, finished_at, duration_seconds)
SELECT p.id, 50001, 'main', 'a1b2c3d4', 'success',
       now() - interval '2 hours', now() - interval '1 hour 52 minutes', 480
FROM projects p WHERE p.gitlab_project_id = 1001
ON CONFLICT (project_id, gitlab_pipeline_id) DO NOTHING;

INSERT INTO pipelines (project_id, gitlab_pipeline_id, ref, sha, status, started_at, finished_at, duration_seconds)
SELECT p.id, 50002, 'feature/cart-fix', 'e5f6a7b8', 'failed',
       now() - interval '30 minutes', now() - interval '24 minutes', 360
FROM projects p WHERE p.gitlab_project_id = 1001
ON CONFLICT (project_id, gitlab_pipeline_id) DO NOTHING;

-- Jobs on the failed pipeline.
INSERT INTO jobs (pipeline_id, gitlab_job_id, name, stage, status, started_at, finished_at, duration_seconds)
SELECT pl.id, 900021, 'unit-tests', 'test', 'failed',
       now() - interval '29 minutes', now() - interval '26 minutes', 180
FROM pipelines pl
JOIN projects p ON p.id = pl.project_id
WHERE p.gitlab_project_id = 1001 AND pl.gitlab_pipeline_id = 50002
ON CONFLICT (pipeline_id, gitlab_job_id) DO NOTHING;

INSERT INTO jobs (pipeline_id, gitlab_job_id, name, stage, status, started_at, finished_at, duration_seconds)
SELECT pl.id, 900022, 'build', 'build', 'success',
       now() - interval '30 minutes', now() - interval '29 minutes', 60
FROM pipelines pl
JOIN projects p ON p.id = pl.project_id
WHERE p.gitlab_project_id = 1001 AND pl.gitlab_pipeline_id = 50002
ON CONFLICT (pipeline_id, gitlab_job_id) DO NOTHING;

COMMIT;
