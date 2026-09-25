-- SPDX-License-Identifier: AGPL-3.0-only
-- Shared project groups (AEON-136). Groups are personal by default and live in a
-- person's preferences; an admin can share one with the whole workspace, which
-- makes it this small entity. A project is in at most one shared group (the
-- primary key on the member row), so everyone sees it in one place. Writes are
-- admin-only in the handlers and append events that can be undone; tenant RLS
-- remains the database boundary.
CREATE TABLE project_groups (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60 AND name = btrim(name)),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by) REFERENCES principals(tenant_id, id)
);
CREATE UNIQUE INDEX project_groups_name_idx ON project_groups(tenant_id, lower(name));
ALTER TABLE project_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_groups FORCE ROW LEVEL SECURITY;
CREATE POLICY project_groups_tenant ON project_groups
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE project_group_members (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL,
    group_id uuid NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, project_id),
    FOREIGN KEY (tenant_id, group_id) REFERENCES project_groups(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, project_id) REFERENCES nodes(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX project_group_members_group_idx ON project_group_members(tenant_id, group_id, project_id);
ALTER TABLE project_group_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_group_members FORCE ROW LEVEL SECURITY;
CREATE POLICY project_group_members_tenant ON project_group_members
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
