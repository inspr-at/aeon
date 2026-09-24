-- SPDX-License-Identifier: AGPL-3.0-only
-- Ephemeral collaboration leases. Draft content and cursor text never enter events.
CREATE TABLE quote_presence (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    session_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    mode text NOT NULL CHECK (mode IN ('viewing','editing','idle')),
    anchor jsonb,
    observed_revision bigint NOT NULL CHECK (observed_revision > 0),
    last_seen timestamptz NOT NULL DEFAULT clock_timestamp(),
    last_interaction timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL DEFAULT (clock_timestamp() + interval '45 seconds'),
    PRIMARY KEY (tenant_id,quote_node_id,session_id),
    FOREIGN KEY (tenant_id,quote_node_id) REFERENCES business_quotes(tenant_id,quote_node_id),
    FOREIGN KEY (tenant_id,principal_id) REFERENCES principals(tenant_id,id),
    CHECK (anchor IS NULL OR jsonb_typeof(anchor)='object')
);
CREATE INDEX quote_presence_live ON quote_presence (tenant_id,quote_node_id,expires_at);
ALTER TABLE quote_presence ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_presence FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_presence_tenant ON quote_presence
    USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
