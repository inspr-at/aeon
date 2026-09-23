-- SPDX-License-Identifier: AGPL-3.0-only
-- Approval rows are projections; approval.proposed/approved/denied events are
-- the durable decision history and are written in the same transaction.
CREATE TABLE approval_requests (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    proposed_by_principal_id uuid NOT NULL,
    agent_principal_id uuid NOT NULL,
    run_id uuid,
    scope text NOT NULL CHECK (scope ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    resource_kind text NOT NULL CHECK (resource_kind IN ('tenant', 'node', 'run')),
    resource_id uuid,
    rationale text NOT NULL CHECK (length(btrim(rationale)) > 0),
    expires_at timestamptz NOT NULL,
    proposed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, proposed_by_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, agent_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id),
    CHECK ((resource_kind = 'tenant') = (resource_id IS NULL)),
    CHECK (expires_at > proposed_at)
);
CREATE INDEX approval_requests_agent_idx ON approval_requests(tenant_id, agent_principal_id, proposed_at DESC);
ALTER TABLE approval_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY approval_requests_tenant ON approval_requests
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE approval_decisions (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    request_id uuid NOT NULL,
    decided_by_principal_id uuid NOT NULL,
    decision text NOT NULL CHECK (decision IN ('approved', 'denied')),
    reason text NOT NULL DEFAULT '',
    decided_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, request_id),
    FOREIGN KEY (tenant_id, request_id) REFERENCES approval_requests(tenant_id, id),
    FOREIGN KEY (tenant_id, decided_by_principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE approval_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_decisions FORCE ROW LEVEL SECURITY;
CREATE POLICY approval_decisions_tenant ON approval_decisions
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_guard_approval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actor_kind text; target_kind text; expiry timestamptz;
BEGIN
    IF TG_TABLE_NAME = 'approval_requests' THEN
        SELECT kind INTO actor_kind FROM principals
            WHERE tenant_id = NEW.tenant_id AND id = NEW.proposed_by_principal_id;
        SELECT kind INTO target_kind FROM principals
            WHERE tenant_id = NEW.tenant_id AND id = NEW.agent_principal_id;
        IF actor_kind <> 'agent' OR target_kind <> 'agent'
           OR NEW.proposed_by_principal_id <> NEW.agent_principal_id THEN
            RAISE EXCEPTION 'only an agent may propose its own permission';
        END IF;
    ELSE
        SELECT kind INTO actor_kind FROM principals
            WHERE tenant_id = NEW.tenant_id AND id = NEW.decided_by_principal_id;
        SELECT expires_at INTO expiry FROM approval_requests
            WHERE tenant_id = NEW.tenant_id AND id = NEW.request_id;
        IF actor_kind <> 'person' OR expiry <= now() THEN
            RAISE EXCEPTION 'only a person may decide a live approval';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER approval_requests_agent_guard BEFORE INSERT ON approval_requests
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_approval();
CREATE TRIGGER approval_decisions_person_guard BEFORE INSERT ON approval_decisions
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_approval();
CREATE TRIGGER approval_requests_immutable BEFORE UPDATE OR DELETE ON approval_requests
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER approval_decisions_immutable BEFORE UPDATE OR DELETE ON approval_decisions
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE agent_permission_grants (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    approval_request_id uuid NOT NULL,
    agent_principal_id uuid NOT NULL,
    scope text NOT NULL,
    resource_kind text NOT NULL,
    resource_id uuid,
    valid_until timestamptz NOT NULL,
    revoked_at timestamptz,
    PRIMARY KEY (tenant_id, approval_request_id),
    FOREIGN KEY (tenant_id, approval_request_id) REFERENCES approval_decisions(tenant_id, request_id),
    FOREIGN KEY (tenant_id, agent_principal_id) REFERENCES principals(tenant_id, id)
);
CREATE INDEX agent_permission_active_idx ON agent_permission_grants
    (tenant_id, agent_principal_id, scope, valid_until) WHERE revoked_at IS NULL;
ALTER TABLE agent_permission_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_permission_grants FORCE ROW LEVEL SECURITY;
CREATE POLICY agent_permission_grants_tenant ON agent_permission_grants
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_permission_grant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM approval_decisions d JOIN approval_requests r
          ON r.tenant_id = d.tenant_id AND r.id = d.request_id
        WHERE d.tenant_id = NEW.tenant_id AND d.request_id = NEW.approval_request_id
          AND d.decision = 'approved' AND r.agent_principal_id = NEW.agent_principal_id
          AND r.scope = NEW.scope AND r.resource_kind = NEW.resource_kind
          AND r.resource_id IS NOT DISTINCT FROM NEW.resource_id
          AND r.expires_at = NEW.valid_until
    ) THEN
        RAISE EXCEPTION 'permission grant must match an approved request';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER agent_permission_grant_guard BEFORE INSERT ON agent_permission_grants
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_permission_grant();
