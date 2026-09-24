-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE attachments (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    node_id uuid NOT NULL,
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
    content_type text NOT NULL,
    size bigint NOT NULL CHECK (size >= 0),
    width integer CHECK (width > 0),
    height integer CHECK (height > 0),
    caption text NOT NULL DEFAULT '',
    position numeric(30,15) NOT NULL DEFAULT 0,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    deleted_at timestamptz,
    source_id text,
    source_attachment_id bigint,
    PRIMARY KEY (tenant_id,id),
    FOREIGN KEY (tenant_id,node_id) REFERENCES nodes(tenant_id,id),
    FOREIGN KEY (tenant_id,created_by) REFERENCES principals(tenant_id,id),
    CHECK ((source_id IS NULL) = (source_attachment_id IS NULL))
);
CREATE INDEX attachments_node_order_idx ON attachments(tenant_id,node_id,position,id) WHERE deleted_at IS NULL;
CREATE INDEX attachments_sha_idx ON attachments(tenant_id,sha256);
CREATE UNIQUE INDEX attachments_import_idx ON attachments(tenant_id,source_id,source_attachment_id) WHERE source_id IS NOT NULL;
ALTER TABLE attachments ENABLE ROW LEVEL SECURITY;
ALTER TABLE attachments FORCE ROW LEVEL SECURITY;
CREATE POLICY attachments_tenant ON attachments
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
