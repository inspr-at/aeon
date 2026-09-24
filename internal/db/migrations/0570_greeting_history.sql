-- SPDX-License-Identifier: AGPL-3.0-only
-- Private, ephemeral greeting rotation state. No tenant events are written.
CREATE TABLE greeting_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    greeting_id text NOT NULL,
    shown_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX greeting_history_recent ON greeting_history (tenant_id, principal_id, id DESC);
ALTER TABLE greeting_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE greeting_history FORCE ROW LEVEL SECURITY;
CREATE POLICY greeting_history_tenant ON greeting_history
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
