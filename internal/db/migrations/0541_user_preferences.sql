-- SPDX-License-Identifier: AGPL-3.0-only
-- Per-person UI preferences (list columns and widths, panel split), keyed by a
-- short client-chosen name such as "list:<project-id>". The value is a small JSON
-- object owned by one principal; tenant RLS remains the database boundary.
CREATE TABLE user_preferences (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    principal_id uuid NOT NULL,
    key text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_.:-]{0,127}$'),
    value jsonb NOT NULL CHECK (jsonb_typeof(value) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, principal_id, key),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE user_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_preferences FORCE ROW LEVEL SECURITY;
CREATE POLICY user_preferences_tenant ON user_preferences
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
