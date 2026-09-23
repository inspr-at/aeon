-- SPDX-License-Identifier: AGPL-3.0-only
-- Periods group one principal's entries; an approved period is a sealed snapshot.
CREATE TABLE time_periods (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'approved')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id),
    CHECK (ends_at > starts_at)
);
CREATE INDEX time_periods_principal_idx ON time_periods(tenant_id, principal_id, starts_at DESC);
ALTER TABLE time_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY time_periods_tenant ON time_periods
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE time_entries (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    period_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    node_id uuid NOT NULL,
    cost_unit_node_id uuid NOT NULL,
    source text NOT NULL CHECK (source IN ('manual', 'agent_run')),
    agent_run_id uuid,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL,
    duration_seconds bigint NOT NULL CHECK (duration_seconds > 0),
    rate_amount numeric(18, 4) NOT NULL CHECK (rate_amount >= 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    amount numeric(18, 4) NOT NULL CHECK (amount >= 0),
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, period_id) REFERENCES time_periods(tenant_id, id),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, cost_unit_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, agent_run_id) REFERENCES agent_runs(tenant_id, id),
    CHECK ((source = 'agent_run') = (agent_run_id IS NOT NULL)),
    CHECK (ended_at > started_at),
    CHECK (duration_seconds = extract(epoch FROM (ended_at - started_at))),
    CHECK (amount = round(rate_amount * duration_seconds / 3600, 4))
);
CREATE UNIQUE INDEX time_entries_agent_run_idx ON time_entries(tenant_id, agent_run_id)
    WHERE agent_run_id IS NOT NULL;
CREATE INDEX time_entries_node_idx ON time_entries(tenant_id, node_id, started_at);
CREATE INDEX time_entries_period_idx ON time_entries(tenant_id, period_id, started_at);
ALTER TABLE time_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY time_entries_tenant ON time_entries
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_time_entry() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p time_periods%ROWTYPE;
BEGIN
    SELECT * INTO p FROM time_periods WHERE tenant_id = NEW.tenant_id AND id = NEW.period_id FOR UPDATE;
    IF NOT FOUND OR p.principal_id <> NEW.principal_id OR p.state <> 'open'
       OR NEW.started_at < p.starts_at OR NEW.ended_at > p.ends_at
       OR NOT aeon_business_node_kind(NEW.tenant_id, NEW.cost_unit_node_id, 'cost_unit') THEN
        RAISE EXCEPTION 'invalid time entry period or cost unit';
    END IF;
    IF NEW.source = 'agent_run' AND NOT EXISTS (
        SELECT 1 FROM agent_runs r WHERE r.tenant_id = NEW.tenant_id AND r.id = NEW.agent_run_id
          AND r.agent_principal_id = NEW.principal_id AND r.work_order_id = NEW.node_id
          AND r.started_at = NEW.started_at AND r.ended_at = NEW.ended_at
          AND r.status IN ('completed', 'failed', 'cancelled')
    ) THEN
        RAISE EXCEPTION 'agent time requires a terminal matching run';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER time_entries_guard BEFORE INSERT ON time_entries
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_time_entry();
CREATE TRIGGER time_entries_immutable BEFORE UPDATE OR DELETE ON time_entries
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE time_period_approvals (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    period_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    approved_by_principal_id uuid NOT NULL,
    entries_sha256 text NOT NULL CHECK (entries_sha256 ~ '^[0-9a-f]{64}$'),
    total_seconds bigint NOT NULL CHECK (total_seconds >= 0),
    approved_at timestamptz NOT NULL DEFAULT now(),
    event_id bigint NOT NULL,
    PRIMARY KEY (tenant_id, period_id),
    UNIQUE (tenant_id, event_id),
    FOREIGN KEY (tenant_id, period_id) REFERENCES time_periods(tenant_id, id),
    FOREIGN KEY (tenant_id, approved_by_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, id)
);
ALTER TABLE time_period_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_period_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY time_period_approvals_tenant ON time_period_approvals
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_time_period_approval() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM time_periods t JOIN principals p
          ON p.tenant_id = NEW.tenant_id AND p.id = NEW.approved_by_principal_id
        JOIN events e ON e.tenant_id = NEW.tenant_id AND e.id = NEW.event_id
        WHERE t.tenant_id = NEW.tenant_id AND t.id = NEW.period_id
          AND t.state = 'open' AND t.revision = NEW.revision
          AND p.kind = 'person' AND 'admin' = ANY(p.roles)
          AND e.type = 'hours.period_approved'
          AND e.actor_principal_id = p.id
    ) THEN
        RAISE EXCEPTION 'period approval requires an admin person and matching event';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER time_period_approvals_guard BEFORE INSERT ON time_period_approvals
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_time_period_approval();
CREATE TRIGGER time_period_approvals_immutable BEFORE UPDATE OR DELETE ON time_period_approvals
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
