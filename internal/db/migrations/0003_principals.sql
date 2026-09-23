CREATE TABLE principals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    kind text NOT NULL CHECK (kind IN ('person','agent')),
    identity_id uuid REFERENCES identities(id),
    name text NOT NULL,
    roles text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX principals_tenant_identity ON principals (tenant_id, identity_id) WHERE identity_id IS NOT NULL;
ALTER TABLE principals ENABLE ROW LEVEL SECURITY;
ALTER TABLE principals FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON principals
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
