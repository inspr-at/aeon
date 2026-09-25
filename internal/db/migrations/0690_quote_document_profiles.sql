-- SPDX-License-Identifier: AGPL-3.0-only
-- EQ1: immutable tenant document profile revisions and their content-addressed assets.
CREATE TABLE quote_document_profiles (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    current_revision integer NOT NULL CHECK (current_revision > 0),
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,id)
);
ALTER TABLE quote_document_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_document_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_document_profiles_tenant ON quote_document_profiles USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_document_profile_revisions (
    tenant_id uuid NOT NULL,
    profile_id uuid NOT NULL,
    revision integer NOT NULL CHECK (revision > 0),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition)='object' AND definition->>'schema'='inspr.document-profile.v1'),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by_principal_id uuid NOT NULL,
    PRIMARY KEY (tenant_id,profile_id,revision),
    FOREIGN KEY (tenant_id,profile_id) REFERENCES quote_document_profiles(tenant_id,id),
    FOREIGN KEY (tenant_id,created_by_principal_id) REFERENCES principals(tenant_id,id)
);
ALTER TABLE quote_document_profile_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_document_profile_revisions FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_document_profile_revisions_tenant ON quote_document_profile_revisions USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE TRIGGER quote_document_profile_revisions_immutable BEFORE UPDATE OR DELETE ON quote_document_profile_revisions FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE quote_document_profile_assets (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    content_type text NOT NULL CHECK (content_type IN ('font/ttf','font/otf','font/woff2','image/png','image/svg+xml')),
    size bigint NOT NULL CHECK (size BETWEEN 1 AND 10485760),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by_principal_id uuid NOT NULL,
    PRIMARY KEY (tenant_id,id),
    FOREIGN KEY (tenant_id,created_by_principal_id) REFERENCES principals(tenant_id,id)
);
ALTER TABLE quote_document_profile_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_document_profile_assets FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_document_profile_assets_tenant ON quote_document_profile_assets USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE TRIGGER quote_document_profile_assets_immutable BEFORE UPDATE OR DELETE ON quote_document_profile_assets FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

ALTER TABLE quote_settings ADD COLUMN default_profile_id uuid;
ALTER TABLE quote_settings ADD CONSTRAINT quote_settings_default_profile_fk FOREIGN KEY (tenant_id,default_profile_id) REFERENCES quote_document_profiles(tenant_id,id);
