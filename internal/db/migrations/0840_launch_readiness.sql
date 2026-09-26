-- SPDX-License-Identifier: AGPL-3.0-only
-- Pharos posts launch_readiness on the deploy handoff. Admission reads that
-- row and the immutable built artifact. The admit body is not evidence.

ALTER TABLE stage_handoff_evidence
    ADD COLUMN reviewed_plan_digest text,
    ADD COLUMN host text,
    ADD COLUMN all_host_eval_passed boolean,
    ADD COLUMN target_build_passed boolean,
    ADD COLUMN backup_ready boolean,
    ADD COLUMN backup_observed_at timestamptz,
    ADD COLUMN restart_required boolean,
    ADD COLUMN running_kernel text,
    ADD COLUMN expected_kernel text;

ALTER TABLE stage_handoff_evidence
    ADD CONSTRAINT stage_handoff_evidence_reviewed_plan_digest_check
    CHECK (reviewed_plan_digest IS NULL OR reviewed_plan_digest ~ '^[0-9a-f]{64}$');

DO $$
DECLARE kind_name text; shape_name text; ceiling_name text;
BEGIN
    SELECT con.conname INTO kind_name
    FROM pg_constraint con
    WHERE con.conrelid = 'stage_handoff_evidence'::regclass
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%kind%deployment%verification%authorization%credential_handoff%'
      AND pg_get_constraintdef(con.oid) NOT ILIKE '%workflow%';
    IF kind_name IS NULL THEN
        RAISE EXCEPTION 'stage_handoff_evidence kind check not found';
    END IF;
    EXECUTE format('ALTER TABLE stage_handoff_evidence DROP CONSTRAINT %I', kind_name);

    SELECT con.conname INTO shape_name
    FROM pg_constraint con
    WHERE con.conrelid = 'stage_handoff_evidence'::regclass
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%credential_ready%'
      AND pg_get_constraintdef(con.oid) ILIKE '%workflow%';
    IF shape_name IS NULL THEN
        RAISE EXCEPTION 'stage_handoff_evidence shape check not found';
    END IF;
    EXECUTE format('ALTER TABLE stage_handoff_evidence DROP CONSTRAINT %I', shape_name);

    SELECT con.conname INTO ceiling_name
    FROM pg_constraint con
    WHERE con.conrelid = 'stage_handoffs'::regclass
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%evidence_ceiling%';
    IF ceiling_name IS NULL THEN
        RAISE EXCEPTION 'stage_handoffs evidence ceiling check not found';
    END IF;
    EXECUTE format('ALTER TABLE stage_handoffs DROP CONSTRAINT %I', ceiling_name);
END $$;

ALTER TABLE stage_handoff_evidence
    ADD CONSTRAINT stage_handoff_evidence_kind_check
    CHECK (kind IN ('deployment', 'verification', 'authorization', 'credential_handoff', 'launch_readiness'));

ALTER TABLE stage_handoff_evidence
    ADD CONSTRAINT stage_handoff_evidence_shape_check
    CHECK (
        (kind IN ('deployment', 'verification')
         AND workflow IS NOT NULL AND environment IS NOT NULL
         AND version_scheme IS NOT NULL AND version IS NOT NULL AND release_channel IS NOT NULL
         AND release_sequence IS NOT NULL AND artifact_digest_sha256 IS NOT NULL
         AND commit_digest IS NOT NULL AND manifest_coordinate IS NOT NULL
         AND manifest_digest_sha256 IS NOT NULL AND authorized IS NULL AND credential_ready IS NULL
         AND reviewed_plan_digest IS NULL AND host IS NULL
         AND all_host_eval_passed IS NULL AND target_build_passed IS NULL AND backup_ready IS NULL
         AND backup_observed_at IS NULL AND restart_required IS NULL
         AND running_kernel IS NULL AND expected_kernel IS NULL)
        OR
        (kind IN ('authorization', 'credential_handoff')
         AND workflow IS NULL AND environment IS NULL
         AND version_scheme IS NULL AND version IS NULL AND release_channel IS NULL
         AND release_sequence IS NULL AND artifact_digest_sha256 IS NULL AND commit_digest IS NULL
         AND manifest_coordinate IS NULL AND manifest_digest_sha256 IS NULL
         AND reviewed_plan_digest IS NULL AND host IS NULL
         AND all_host_eval_passed IS NULL AND target_build_passed IS NULL AND backup_ready IS NULL
         AND backup_observed_at IS NULL AND restart_required IS NULL
         AND running_kernel IS NULL AND expected_kernel IS NULL
         AND ((kind = 'authorization' AND authorized IS NOT NULL AND credential_ready IS NULL)
           OR (kind = 'credential_handoff' AND authorized IS NULL AND credential_ready IS NOT NULL)))
        OR
        (kind = 'launch_readiness'
         AND workflow IS NULL AND environment IS NULL
         AND version_scheme IS NULL AND version IS NULL AND release_channel IS NULL
         AND release_sequence IS NULL AND artifact_digest_sha256 IS NULL AND commit_digest IS NULL
         AND manifest_coordinate IS NULL AND manifest_digest_sha256 IS NULL
         AND authorized IS NULL AND credential_ready IS NULL
         AND reviewed_plan_digest IS NOT NULL
         AND host IS NOT NULL AND length(btrim(host)) BETWEEN 1 AND 256
         AND all_host_eval_passed IS NOT NULL AND target_build_passed IS NOT NULL
         AND backup_ready IS NOT NULL AND backup_observed_at IS NOT NULL
         AND restart_required IS NOT NULL
         AND running_kernel IS NOT NULL AND length(btrim(running_kernel)) BETWEEN 1 AND 128
         AND expected_kernel IS NOT NULL AND length(btrim(expected_kernel)) BETWEEN 1 AND 128)
    );

ALTER TABLE stage_handoffs
    ADD CONSTRAINT stage_handoffs_evidence_ceiling_check
    CHECK (
        cardinality(evidence_ceiling) > 0 AND
        array_position(evidence_ceiling, NULL) IS NULL AND
        evidence_ceiling <@ ARRAY['deployment', 'verification', 'authorization', 'credential_handoff', 'launch_readiness']::text[]
    );
