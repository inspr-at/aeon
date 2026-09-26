-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-172: exact launch request receipts. No cross-tenant data rewrite is
-- needed, so this migration also runs as the NOBYPASSRLS application owner.
ALTER TABLE stage_launch_admissions
    ADD COLUMN consumed_by_principal_id uuid;
ALTER TABLE stage_launch_admissions
    ADD CONSTRAINT stage_launch_admissions_consumer_fk
    FOREIGN KEY (tenant_id, consumed_by_principal_id)
    REFERENCES principals(tenant_id, id);

CREATE TABLE stage_launch_request_receipts (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    handoff_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN ('admit', 'consume')),
    principal_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    body_digest_sha256 text NOT NULL CHECK (body_digest_sha256 ~ '^[0-9a-f]{64}$'),
    response jsonb NOT NULL CHECK (jsonb_typeof(response) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, handoff_id, idempotency_key),
    FOREIGN KEY (tenant_id, handoff_id) REFERENCES stage_handoffs(tenant_id, id),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE stage_launch_request_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_launch_request_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stage_launch_request_receipts
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE TRIGGER stage_launch_request_receipts_immutable
    BEFORE UPDATE OR DELETE ON stage_launch_request_receipts
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
