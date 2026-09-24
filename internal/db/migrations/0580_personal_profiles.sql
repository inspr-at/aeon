-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE personal_profiles (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    principal_id uuid NOT NULL,
    first_name text NOT NULL DEFAULT '',
    last_name text NOT NULL DEFAULT '',
    preferred_name text NOT NULL DEFAULT '',
    short_name text NOT NULL DEFAULT '',
    initials_override text NOT NULL DEFAULT '',
    timezone text NOT NULL DEFAULT 'Europe/Vienna',
    locale text NOT NULL DEFAULT 'de-AT',
    greeting_enabled boolean NOT NULL DEFAULT true,
    avatar_original_hash text NOT NULL DEFAULT '',
    avatar_hashes jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, principal_id),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id),
    CHECK (short_name = '' OR short_name ~ '^[a-z0-9._-]{2,24}$'),
    CHECK (char_length(initials_override) <= 3),
    CHECK (avatar_original_hash = '' OR avatar_original_hash ~ '^[a-f0-9]{64}$'),
    CHECK (jsonb_typeof(avatar_hashes) = 'object')
);
CREATE UNIQUE INDEX personal_profiles_short_name ON personal_profiles (tenant_id, short_name) WHERE short_name <> '';
ALTER TABLE personal_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE personal_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY personal_profiles_tenant ON personal_profiles
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
