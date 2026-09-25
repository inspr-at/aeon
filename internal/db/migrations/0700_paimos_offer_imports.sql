-- SPDX-License-Identifier: AGPL-3.0-only
-- One foreign identity maps to one tenant node. Source revisions are distinct
-- from Aeon quote versions and only strictly newer revisions may replace it.
CREATE TABLE paimos_offer_imports (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    source_system text NOT NULL DEFAULT 'paimos' CHECK (source_system = 'paimos'),
    source_instance text NOT NULL CHECK (length(source_instance) BETWEEN 1 AND 100),
    source_kind text NOT NULL CHECK (source_kind IN ('customer','contact','offer')),
    source_id text NOT NULL CHECK (length(source_id) BETWEEN 1 AND 100),
    node_id uuid NOT NULL,
    source_number text NOT NULL DEFAULT '',
    source_revision text NOT NULL,
    revision_rank bigint NOT NULL CHECK (revision_rank >= 0),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    imported_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id,source_instance,source_kind,source_id),
    UNIQUE (tenant_id,node_id),
    FOREIGN KEY (tenant_id,node_id) REFERENCES nodes(tenant_id,id)
);
ALTER TABLE paimos_offer_imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE paimos_offer_imports FORCE ROW LEVEL SECURITY;
CREATE POLICY paimos_offer_imports_tenant ON paimos_offer_imports
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
