-- SPDX-License-Identifier: AGPL-3.0-only

-- Agent API keys. Bearer tokens are aeon_<prefix>_<secret>. The prefix begins
-- with the tenant uuid as 32 hex characters so resolution can enter that
-- tenant and read this table under row-level security. hash is the hex sha256
-- of the secret component. The secret itself is shown once at creation.
CREATE TABLE agent_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants (id),
    principal_id uuid NOT NULL REFERENCES principals (id),
    name text NOT NULL,
    prefix text NOT NULL,
    hash text NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz
);

CREATE UNIQUE INDEX agent_keys_prefix ON agent_keys (prefix);

ALTER TABLE agent_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_keys FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON agent_keys
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
