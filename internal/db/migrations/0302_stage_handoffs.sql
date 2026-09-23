-- SPDX-License-Identifier: AGPL-3.0-only
-- One fenced request -> typed evidence -> terminal result contract for stage plugins.
CREATE TABLE stage_handoffs (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    project_node_id uuid NOT NULL,
    release_node_id uuid NOT NULL,
    stage text NOT NULL CHECK (stage IN ('deploy', 'access')),
    operation text NOT NULL CHECK (operation IN ('prepare', 'apply', 'deploy', 'verify')),
    plugin_id text NOT NULL CHECK (plugin_id ~ '^[a-z][a-z0-9_]*$'),
    requested_by_principal_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    attempt integer NOT NULL CHECK (attempt > 0),
    authority_epoch bigint NOT NULL CHECK (authority_epoch > 0),
    journey_revision bigint NOT NULL CHECK (journey_revision > 0),
    plan_digest text NOT NULL CHECK (plan_digest ~ '^[0-9a-f]{64}$'),
    predecessor_digest text NOT NULL CHECK (predecessor_digest ~ '^[0-9a-f]{64}$'),
    context_digest text NOT NULL CHECK (context_digest ~ '^[0-9a-f]{64}$'),
    prerequisite_seal_sha256 text NOT NULL CHECK (prerequisite_seal_sha256 ~ '^[0-9a-f]{64}$'),
    evidence_ceiling text[] NOT NULL CHECK (
        cardinality(evidence_ceiling) > 0 AND
        array_position(evidence_ceiling, NULL) IS NULL AND
        evidence_ceiling <@ ARRAY['deployment', 'verification', 'authorization', 'credential_handoff']::text[]),
    state text NOT NULL DEFAULT 'requested' CHECK (state IN ('requested', 'active', 'blocked', 'succeeded', 'failed', 'revoked')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, project_node_id, stage, idempotency_key),
    UNIQUE (tenant_id, release_node_id, stage, operation, attempt),
    FOREIGN KEY (tenant_id, project_node_id, release_node_id)
        REFERENCES journey_releases(tenant_id, project_node_id, release_node_id),
    FOREIGN KEY (tenant_id, requested_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK (expires_at > created_at),
    CHECK ((stage = 'access' AND operation IN ('prepare', 'apply')) OR
           (stage = 'deploy' AND operation IN ('deploy', 'verify')))
);
CREATE INDEX stage_handoffs_release_idx ON stage_handoffs(tenant_id, release_node_id, stage, operation, attempt DESC);
ALTER TABLE stage_handoffs ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_handoffs FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_handoffs_tenant ON stage_handoffs
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- A required dependency belongs to the same release; its current terminal result
-- must still match this sealed set before the owner result may succeed.
CREATE TABLE stage_handoff_dependencies (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    handoff_id uuid NOT NULL,
    dependency_handoff_id uuid NOT NULL,
    required_kind text NOT NULL CHECK (required_kind IN ('deployment', 'verification', 'authorization', 'credential_handoff')),
    PRIMARY KEY (tenant_id, handoff_id, dependency_handoff_id, required_kind),
    FOREIGN KEY (tenant_id, handoff_id) REFERENCES stage_handoffs(tenant_id, id),
    FOREIGN KEY (tenant_id, dependency_handoff_id) REFERENCES stage_handoffs(tenant_id, id),
    CHECK (handoff_id <> dependency_handoff_id)
);
ALTER TABLE stage_handoff_dependencies ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_handoff_dependencies FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_handoff_dependencies_tenant ON stage_handoff_dependencies
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE stage_handoff_evidence (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    handoff_id uuid NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    authority_epoch bigint NOT NULL CHECK (authority_epoch > 0),
    kind text NOT NULL CHECK (kind IN ('deployment', 'verification', 'authorization', 'credential_handoff')),
    outcome text NOT NULL CHECK (outcome IN ('succeeded', 'failed', 'satisfied', 'blocked')),
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    workflow text,
    environment text,
    version_scheme text CHECK (version_scheme IN ('legacy', 'inspr-calendar-v1', 'inspr-calendar-v2')),
    version text,
    release_channel text,
    release_sequence bigint CHECK (release_sequence > 0),
    artifact_digest_sha256 text CHECK (artifact_digest_sha256 ~ '^[0-9a-f]{64}$'),
    commit_digest text,
    manifest_coordinate text,
    manifest_digest_sha256 text CHECK (manifest_digest_sha256 ~ '^[0-9a-f]{64}$'),
    authorized boolean,
    credential_ready boolean,
    PRIMARY KEY (tenant_id, handoff_id, sequence),
    FOREIGN KEY (tenant_id, handoff_id) REFERENCES stage_handoffs(tenant_id, id),
    CHECK (
        (kind IN ('deployment', 'verification') AND workflow IS NOT NULL AND environment IS NOT NULL
         AND version_scheme IS NOT NULL AND version IS NOT NULL AND release_channel IS NOT NULL
         AND release_sequence IS NOT NULL AND artifact_digest_sha256 IS NOT NULL
         AND commit_digest IS NOT NULL AND manifest_coordinate IS NOT NULL
         AND manifest_digest_sha256 IS NOT NULL AND authorized IS NULL AND credential_ready IS NULL)
        OR
        (kind IN ('authorization', 'credential_handoff') AND workflow IS NULL AND environment IS NULL
         AND version_scheme IS NULL AND version IS NULL AND release_channel IS NULL
         AND release_sequence IS NULL AND artifact_digest_sha256 IS NULL AND commit_digest IS NULL
         AND manifest_coordinate IS NULL AND manifest_digest_sha256 IS NULL
         AND ((kind = 'authorization' AND authorized IS NOT NULL AND credential_ready IS NULL)
           OR (kind = 'credential_handoff' AND authorized IS NULL AND credential_ready IS NOT NULL)))
    )
);
ALTER TABLE stage_handoff_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_handoff_evidence FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_handoff_evidence_tenant ON stage_handoff_evidence
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE stage_handoff_results (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    handoff_id uuid NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
    terminal_sequence bigint NOT NULL,
    authority_epoch bigint NOT NULL CHECK (authority_epoch > 0),
    prerequisite_seal_sha256 text NOT NULL CHECK (prerequisite_seal_sha256 ~ '^[0-9a-f]{64}$'),
    blocker_code text CHECK (blocker_code IN
        ('dependency_pending', 'dependency_failed', 'reporter_stale', 'external_waiting', 'policy_refused')),
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, handoff_id),
    FOREIGN KEY (tenant_id, handoff_id, terminal_sequence)
        REFERENCES stage_handoff_evidence(tenant_id, handoff_id, sequence),
    CHECK ((outcome = 'succeeded') = (blocker_code IS NULL))
);
ALTER TABLE stage_handoff_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_handoff_results FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_handoff_results_tenant ON stage_handoff_results
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- Pharos consumes one reviewed launch admission before changing a host. It is
-- bound to the same artifact, plan and authority epoch as the handoff.
CREATE TABLE stage_launch_admissions (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    handoff_id uuid NOT NULL,
    binding_digest_sha256 text NOT NULL CHECK (binding_digest_sha256 ~ '^[0-9a-f]{64}$'),
    artifact_digest_sha256 text NOT NULL CHECK (artifact_digest_sha256 ~ '^[0-9a-f]{64}$'),
    authority_epoch bigint NOT NULL CHECK (authority_epoch > 0),
    max_uses integer NOT NULL DEFAULT 1 CHECK (max_uses = 1),
    consumed_at timestamptz,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, handoff_id, binding_digest_sha256),
    FOREIGN KEY (tenant_id, handoff_id) REFERENCES stage_handoffs(tenant_id, id)
);
ALTER TABLE stage_launch_admissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_launch_admissions FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_launch_admissions_tenant ON stage_launch_admissions
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TRIGGER stage_handoff_dependencies_immutable BEFORE UPDATE OR DELETE ON stage_handoff_dependencies
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER stage_handoff_evidence_immutable BEFORE UPDATE OR DELETE ON stage_handoff_evidence
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER stage_handoff_results_immutable BEFORE UPDATE OR DELETE ON stage_handoff_results
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
