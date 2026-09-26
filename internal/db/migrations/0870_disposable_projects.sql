-- SPDX-License-Identifier: AGPL-3.0-only
-- An operator can explicitly mark a project in a development environment.
-- The marker is separate from mutable node fields and cannot be set by the API.
CREATE TABLE journey_disposable_projects (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_node_id uuid NOT NULL,
    marked_by_principal_id uuid NOT NULL,
    marked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, marked_by_principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE journey_disposable_projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_disposable_projects FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_disposable_projects_tenant ON journey_disposable_projects
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
