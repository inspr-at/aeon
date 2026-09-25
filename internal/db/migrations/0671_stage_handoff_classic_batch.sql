-- SPDX-License-Identifier: AGPL-3.0-only
-- A classic batch number is provenance for an Aeon stage handoff, not a batch resource.
CREATE TABLE stage_handoff_classic_batch_aliases (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_node_id uuid NOT NULL,
    classic_batch_id bigint NOT NULL CHECK (classic_batch_id > 0),
    classic_project_id bigint NOT NULL CHECK (classic_project_id > 0),
    handoff_id uuid NOT NULL,
    classic_batch jsonb NOT NULL CHECK (jsonb_typeof(classic_batch)='object'),
    implementation_execution bigint NOT NULL DEFAULT 0 CHECK (implementation_execution >= 0),
    implementation_authority_epoch bigint NOT NULL DEFAULT 0 CHECK (implementation_authority_epoch >= 0),
    account_key text NOT NULL DEFAULT '',
    runtime_generation text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, project_node_id, classic_batch_id),
    UNIQUE (tenant_id, handoff_id),
    FOREIGN KEY (tenant_id, handoff_id) REFERENCES stage_handoffs(tenant_id, id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES nodes(tenant_id, id),
    CHECK ((implementation_execution=0)=(implementation_authority_epoch=0)),
    CHECK (classic_batch->>'id'=classic_batch_id::text AND classic_batch->>'project_id'=classic_project_id::text)
);
ALTER TABLE stage_handoff_classic_batch_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_handoff_classic_batch_aliases FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_handoff_classic_batch_aliases_tenant ON stage_handoff_classic_batch_aliases
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE TRIGGER stage_handoff_classic_batch_aliases_immutable BEFORE UPDATE OR DELETE ON stage_handoff_classic_batch_aliases
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

-- Typed implementation/QA evidence belongs to the aliased handoff attempt.
-- Its single immutable row permits exact idempotent replay by the CLI.
CREATE TABLE stage_handoff_build_evidence (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    handoff_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 80),
    receipt jsonb NOT NULL CHECK (jsonb_typeof(receipt)='object'),
    commit_digest text NOT NULL,
    oci_config_digest text NOT NULL CHECK (oci_config_digest ~ '^[0-9a-f]{64}$'),
    release_manifest_digest text NOT NULL CHECK (release_manifest_digest ~ '^[0-9a-f]{64}$'),
    release_manifest_coordinate text NOT NULL,
    oci_index_digest text NOT NULL DEFAULT '' CHECK (oci_index_digest='' OR oci_index_digest ~ '^[0-9a-f]{64}$'),
    version_scheme text NOT NULL CHECK (version_scheme IN ('legacy','inspr-calendar-v1','inspr-calendar-v2')),
    release_channel text NOT NULL,
    release_sequence bigint NOT NULL CHECK (release_sequence >= 0),
    version text NOT NULL,
    qa_digest text NOT NULL CHECK (qa_digest ~ '^[0-9a-f]{64}$'),
    observed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, handoff_id),
    FOREIGN KEY (tenant_id, handoff_id) REFERENCES stage_handoffs(tenant_id, id)
);
ALTER TABLE stage_handoff_build_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_handoff_build_evidence FORCE ROW LEVEL SECURITY;
CREATE POLICY stage_handoff_build_evidence_tenant ON stage_handoff_build_evidence
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE TRIGGER stage_handoff_build_evidence_immutable BEFORE UPDATE OR DELETE ON stage_handoff_build_evidence
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
