-- SPDX-License-Identifier: AGPL-3.0-only
-- Tenant enablement is a projection of first-party, compiled Go manifests.
-- This table grants no capability absent from the binary's matching digest.
CREATE TABLE plugin_installations (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    plugin_id text NOT NULL CHECK (plugin_id ~ '^[a-z][a-z0-9_]*$'),
    version text NOT NULL CHECK (length(btrim(version)) > 0),
    manifest_digest_sha256 text NOT NULL CHECK (manifest_digest_sha256 ~ '^[0-9a-f]{64}$'),
    owner text NOT NULL CHECK (length(btrim(owner)) > 0),
    enabled boolean NOT NULL DEFAULT false,
    permissions text[] NOT NULL DEFAULT '{}'::text[]
        CHECK (array_position(permissions, NULL) IS NULL),
    updated_by_principal_id uuid NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, plugin_id),
    FOREIGN KEY (tenant_id, updated_by_principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE plugin_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE plugin_installations FORCE ROW LEVEL SECURITY;
CREATE POLICY plugin_installations_tenant ON plugin_installations
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE plugin_installation_events (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    plugin_id text NOT NULL,
    event_id bigint NOT NULL,
    manifest_digest_sha256 text NOT NULL CHECK (manifest_digest_sha256 ~ '^[0-9a-f]{64}$'),
    enabled boolean NOT NULL,
    permissions text[] NOT NULL CHECK (array_position(permissions, NULL) IS NULL),
    PRIMARY KEY (tenant_id, plugin_id, event_id),
    FOREIGN KEY (tenant_id, plugin_id) REFERENCES plugin_installations(tenant_id, plugin_id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, id)
);
ALTER TABLE plugin_installation_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE plugin_installation_events FORCE ROW LEVEL SECURITY;
CREATE POLICY plugin_installation_events_tenant ON plugin_installation_events
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE TRIGGER plugin_installation_events_immutable BEFORE UPDATE OR DELETE ON plugin_installation_events
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
