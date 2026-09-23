-- SPDX-License-Identifier: AGPL-3.0-only
-- Work orders are nodes of the tenant's work_order kind, with typed R2 detail.
CREATE FUNCTION aeon_seed_work_order_kind(p_tenant_id uuid) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
    IF p_tenant_id IS DISTINCT FROM NULLIF(current_setting('aeon.tenant_id', true), '')::uuid THEN
        RAISE EXCEPTION 'tenant setting does not match work-order kind tenant';
    END IF;
    INSERT INTO node_kinds(tenant_id, slug, label, short_prefix, icon, field_schema)
    VALUES (p_tenant_id, 'work_order', 'Work order', 'WOR', 'work-order', '{}'::jsonb)
    ON CONFLICT (tenant_id, slug) DO NOTHING;
END;
$$;
CREATE FUNCTION aeon_seed_new_tenant_work_order_kind() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    PERFORM set_config('aeon.tenant_id', NEW.id::text, true);
    PERFORM aeon_seed_work_order_kind(NEW.id);
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
    RETURN NEW;
EXCEPTION WHEN OTHERS THEN
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
    RAISE;
END;
$$;
CREATE TRIGGER tenants_seed_work_order_kind AFTER INSERT ON tenants
    FOR EACH ROW EXECUTE FUNCTION aeon_seed_new_tenant_work_order_kind();
DO $$
DECLARE t record; prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    FOR t IN SELECT id FROM tenants LOOP
        PERFORM set_config('aeon.tenant_id', t.id::text, true);
        PERFORM aeon_seed_work_order_kind(t.id);
    END LOOP;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
END;
$$;

CREATE TABLE work_orders (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    node_id uuid NOT NULL,
    requested_by_principal_id uuid NOT NULL,
    assignee_principal_id uuid,
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'ready', 'running', 'blocked', 'done', 'cancelled')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    max_cost_micros bigint CHECK (max_cost_micros >= 0),
    max_duration_seconds bigint CHECK (max_duration_seconds > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, node_id),
    FOREIGN KEY (tenant_id, node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, requested_by_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, assignee_principal_id) REFERENCES principals(tenant_id, id)
);
CREATE INDEX work_orders_assignee_idx ON work_orders(tenant_id, assignee_principal_id, status);
ALTER TABLE work_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_orders FORCE ROW LEVEL SECURITY;
CREATE POLICY work_orders_tenant ON work_orders
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_work_order_kind() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM nodes n JOIN node_kinds k
        ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
        WHERE n.tenant_id = NEW.tenant_id AND n.id = NEW.node_id
          AND n.deleted_at IS NULL AND k.slug = 'work_order') THEN
        RAISE EXCEPTION 'work order requires a live work_order node';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER work_orders_kind_guard BEFORE INSERT OR UPDATE OF node_id ON work_orders
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_work_order_kind();

CREATE TABLE work_criteria (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    work_order_id uuid NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    description text NOT NULL CHECK (length(btrim(description)) > 0),
    checked_at timestamptz,
    checked_by_principal_id uuid,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, work_order_id, id),
    UNIQUE (tenant_id, work_order_id, position),
    FOREIGN KEY (tenant_id, work_order_id) REFERENCES work_orders(tenant_id, node_id),
    FOREIGN KEY (tenant_id, checked_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK ((checked_at IS NULL) = (checked_by_principal_id IS NULL))
);
ALTER TABLE work_criteria ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_criteria FORCE ROW LEVEL SECURITY;
CREATE POLICY work_criteria_tenant ON work_criteria
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE agent_runs (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    work_order_id uuid NOT NULL,
    agent_principal_id uuid NOT NULL,
    model_profile_id uuid,
    account_id uuid,
    daemon_id text CHECK (daemon_id IS NULL OR length(daemon_id) BETWEEN 1 AND 128),
    daemon_generation text CHECK (daemon_generation IS NULL OR length(daemon_generation) BETWEEN 1 AND 128),
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'starting', 'running', 'waiting', 'completed', 'failed', 'cancelled', 'ownership_lost')),
    requested_model text CHECK (requested_model IS NULL OR length(requested_model) BETWEEN 1 AND 128),
    effective_model text CHECK (effective_model IS NULL OR length(effective_model) BETWEEN 1 AND 128),
    model_evidence text NOT NULL DEFAULT 'unverified'
        CHECK (model_evidence IN ('unverified', 'vendor_reported')),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cost_micros bigint NOT NULL DEFAULT 0 CHECK (cost_micros >= 0),
    started_at timestamptz,
    ended_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, work_order_id, id),
    FOREIGN KEY (tenant_id, work_order_id) REFERENCES work_orders(tenant_id, node_id),
    FOREIGN KEY (tenant_id, agent_principal_id) REFERENCES principals(tenant_id, id),
    CHECK (ended_at IS NULL OR started_at IS NOT NULL),
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE INDEX agent_runs_order_idx ON agent_runs(tenant_id, work_order_id, created_at DESC);
CREATE INDEX agent_runs_agent_idx ON agent_runs(tenant_id, agent_principal_id, status);
ALTER TABLE agent_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY agent_runs_tenant ON agent_runs
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_agent_run_principal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM principals p WHERE p.tenant_id = NEW.tenant_id
                   AND p.id = NEW.agent_principal_id AND p.kind = 'agent') THEN
        RAISE EXCEPTION 'run principal must be an agent';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER agent_runs_principal_guard BEFORE INSERT OR UPDATE OF agent_principal_id
    ON agent_runs FOR EACH ROW EXECUTE FUNCTION aeon_guard_agent_run_principal();

CREATE TABLE run_telemetry (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    run_id uuid NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    kind text NOT NULL CHECK (kind IN ('started', 'heartbeat', 'turn', 'tool', 'usage', 'status', 'finished')),
    input_tokens_delta bigint NOT NULL DEFAULT 0 CHECK (input_tokens_delta >= 0),
    output_tokens_delta bigint NOT NULL DEFAULT 0 CHECK (output_tokens_delta >= 0),
    cost_micros_delta bigint NOT NULL DEFAULT 0 CHECK (cost_micros_delta >= 0),
    tool_count_delta integer NOT NULL DEFAULT 0 CHECK (tool_count_delta >= 0),
    turn_count_delta integer NOT NULL DEFAULT 0 CHECK (turn_count_delta >= 0),
    at timestamptz NOT NULL DEFAULT now(),
    error_code text CHECK (error_code IN ('event_stream_bound', 'app_server_protocol',
        'child_exit_failed', 'turn_failed', 'child_stop_failed', 'ownership_lost',
        'reporter_unavailable', 'workspace_conflict', 'decision_refused')),
    PRIMARY KEY (tenant_id, run_id, sequence),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id)
);
ALTER TABLE run_telemetry ENABLE ROW LEVEL SECURITY;
ALTER TABLE run_telemetry FORCE ROW LEVEL SECURITY;
CREATE POLICY run_telemetry_tenant ON run_telemetry
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE work_evidence (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    work_order_id uuid NOT NULL,
    criterion_id uuid,
    run_id uuid,
    submitted_by_principal_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('url', 'node', 'text')),
    reference text NOT NULL CHECK (length(btrim(reference)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, work_order_id) REFERENCES work_orders(tenant_id, node_id),
    FOREIGN KEY (tenant_id, work_order_id, criterion_id)
        REFERENCES work_criteria(tenant_id, work_order_id, id),
    FOREIGN KEY (tenant_id, work_order_id, run_id)
        REFERENCES agent_runs(tenant_id, work_order_id, id),
    FOREIGN KEY (tenant_id, submitted_by_principal_id) REFERENCES principals(tenant_id, id)
);
CREATE INDEX work_evidence_order_idx ON work_evidence(tenant_id, work_order_id, created_at);
ALTER TABLE work_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_evidence FORCE ROW LEVEL SECURITY;
CREATE POLICY work_evidence_tenant ON work_evidence
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
