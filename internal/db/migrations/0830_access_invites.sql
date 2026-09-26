-- SPDX-License-Identifier: AGPL-3.0-only
-- P3 access: invites and dot-notation key scopes.
-- Project-table RLS stays with P2. These tables are tenant-scoped only.
-- Invite status is derived: accepted, revoked, expired (pending and past
-- expires_at), otherwise pending. The join token is stored only as a hash.

CREATE TABLE invites (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    email text NOT NULL,
    workspace_role_id uuid,
    token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    accepted_at timestamptz,
    accepted_by uuid,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, workspace_role_id) REFERENCES roles(tenant_id, id),
    FOREIGN KEY (tenant_id, created_by) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, accepted_by) REFERENCES principals(tenant_id, id),
    CONSTRAINT invites_email CHECK (email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$' AND length(email) <= 320),
    CONSTRAINT invites_token CHECK (octet_length(token_hash) = 32),
    CONSTRAINT invites_expiry CHECK (expires_at > created_at),
    CONSTRAINT invites_accepted CHECK ((accepted_at IS NULL) = (accepted_by IS NULL)),
    CONSTRAINT invites_terminal CHECK (revoked_at IS NULL OR accepted_at IS NULL)
);
CREATE TABLE invite_project_roles (
    tenant_id uuid NOT NULL,
    invite_id uuid NOT NULL,
    project_id uuid NOT NULL,
    role_id uuid NOT NULL,
    PRIMARY KEY (tenant_id, invite_id, project_id),
    FOREIGN KEY (tenant_id, invite_id) REFERENCES invites(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, role_id) REFERENCES roles(tenant_id, id),
    FOREIGN KEY (tenant_id, project_id) REFERENCES nodes(tenant_id, id)
);
CREATE INDEX invites_pending_email ON invites (tenant_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

ALTER TABLE invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE invites FORCE ROW LEVEL SECURITY;
CREATE POLICY invites_tenant ON invites
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
ALTER TABLE invite_project_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE invite_project_roles FORCE ROW LEVEL SECURITY;
CREATE POLICY invite_project_roles_tenant ON invite_project_roles
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- Keys are stored in dot notation. Colon input from older rows is normalized
-- in place; matching still accepts either form.
DO $$
DECLARE
    tenant uuid;
    prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        UPDATE agent_keys k SET scopes = (
            SELECT coalesce(array_agg(n.scope ORDER BY n.ord), '{}')
            FROM (
                SELECT DISTINCT ON (replace(s, ':', '.')) replace(s, ':', '.') AS scope, ord
                FROM unnest(k.scopes) WITH ORDINALITY AS u(s, ord)
                ORDER BY replace(s, ':', '.'), ord
            ) n
        )
        WHERE k.tenant_id = tenant AND EXISTS (
            SELECT 1 FROM unnest(k.scopes) AS u(s) WHERE position(':' IN s) > 0
        );
    END LOOP;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
END;
$$;
