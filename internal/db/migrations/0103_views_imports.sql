-- SPDX-License-Identifier: AGPL-3.0-only
-- Saved list views and import status. Owner/share authorization is in handlers;
-- tenant RLS remains the database boundary.
CREATE TABLE saved_views (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    owner_principal_id uuid NOT NULL,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    filters jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(filters) = 'object'),
    sort_field text NOT NULL DEFAULT 'position'
        CHECK (sort_field IN ('position', 'updated_at', 'created_at', 'key', 'title')),
    sort_direction text NOT NULL DEFAULT 'asc' CHECK (sort_direction IN ('asc', 'desc')),
    columns text[] NOT NULL DEFAULT ARRAY['key', 'title', 'state']::text[],
    shared boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, owner_principal_id) REFERENCES principals(tenant_id, id),
    CHECK (array_position(columns, NULL) IS NULL)
);
CREATE INDEX saved_views_owner_idx ON saved_views(tenant_id, owner_principal_id, updated_at DESC);
CREATE INDEX saved_views_shared_idx ON saved_views(tenant_id, updated_at DESC) WHERE shared;
ALTER TABLE saved_views ENABLE ROW LEVEL SECURITY;
ALTER TABLE saved_views FORCE ROW LEVEL SECURITY;
CREATE POLICY saved_views_tenant ON saved_views
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE import_jobs (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    created_by_principal_id uuid NOT NULL,
    source text NOT NULL CHECK (length(btrim(source)) > 0),
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    processed_count bigint NOT NULL DEFAULT 0 CHECK (processed_count >= 0),
    created_count bigint NOT NULL DEFAULT 0 CHECK (created_count >= 0),
    error_count bigint NOT NULL DEFAULT 0 CHECK (error_count >= 0),
    failure_summary text,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK (created_count + error_count <= processed_count)
);
CREATE INDEX import_jobs_created_idx ON import_jobs(tenant_id, created_at DESC, id);
ALTER TABLE import_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY import_jobs_tenant ON import_jobs
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
