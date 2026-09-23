-- SPDX-License-Identifier: AGPL-3.0-only
-- Tenant model policy and opaque local account pool. No vendor credentials.
CREATE TABLE model_profiles (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    slug text NOT NULL CHECK (slug ~ '^[a-z][a-z0-9_-]*$'),
    version text NOT NULL CHECK (length(version) BETWEEN 1 AND 64),
    harness text NOT NULL CHECK (harness IN ('codex', 'claude', 'pi', 'cursor', 'grok')),
    family text NOT NULL CHECK (family IN ('openai', 'anthropic', 'xai', 'cursor')),
    model text NOT NULL CHECK (length(model) BETWEEN 1 AND 128),
    effort text NOT NULL CHECK (length(effort) BETWEEN 1 AND 32),
    tier text NOT NULL CHECK (tier IN ('fast', 'standard', 'strong', 'frontier')),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, slug, version)
);
ALTER TABLE model_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY model_profiles_tenant ON model_profiles
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE TRIGGER model_profiles_immutable BEFORE UPDATE OR DELETE ON model_profiles
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE model_role_routes (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    role text NOT NULL CHECK (role IN ('scout', 'mechanical', 'build', 'build-hard', 'review-gate')),
    priority integer NOT NULL CHECK (priority > 0),
    profile_id uuid NOT NULL,
    valid_until timestamptz,
    state text NOT NULL DEFAULT 'available'
        CHECK (state IN ('available', 'unavailable', 'conserved', 'budget_limited')),
    reason text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, role, priority),
    UNIQUE (tenant_id, role, profile_id),
    FOREIGN KEY (tenant_id, profile_id) REFERENCES model_profiles(tenant_id, id),
    CHECK (state = 'available' OR (valid_until IS NOT NULL AND length(btrim(reason)) > 0))
);
ALTER TABLE model_role_routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_role_routes FORCE ROW LEVEL SECURITY;
CREATE POLICY model_role_routes_tenant ON model_role_routes
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE agent_accounts (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_key text NOT NULL CHECK (account_key ~ '^[a-zA-Z0-9][a-zA-Z0-9._:-]*$' AND length(account_key) <= 128),
    harness text NOT NULL CHECK (harness IN ('codex', 'claude', 'pi', 'cursor', 'grok')),
    daemon_id text NOT NULL CHECK (length(daemon_id) BETWEEN 1 AND 128),
    registered_by_principal_id uuid NOT NULL,
    label text NOT NULL CHECK (length(label) BETWEEN 1 AND 128),
    state text NOT NULL DEFAULT 'available' CHECK (state IN ('available', 'draining', 'unavailable')),
    max_parallel_runs integer NOT NULL DEFAULT 1 CHECK (max_parallel_runs > 0),
    last_probe_at timestamptz,
    last_probe_ok boolean,
    last_daemon_generation text CHECK (last_daemon_generation IS NULL OR
        length(last_daemon_generation) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, daemon_id, harness, account_key),
    FOREIGN KEY (tenant_id, registered_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK ((last_probe_at IS NULL) = (last_probe_ok IS NULL)),
    CHECK ((last_probe_at IS NULL) = (last_daemon_generation IS NULL))
);
CREATE INDEX agent_accounts_route_idx ON agent_accounts(tenant_id, harness, state, id);
ALTER TABLE agent_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY agent_accounts_tenant ON agent_accounts
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_agent_account_registrant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM principals p WHERE p.tenant_id = NEW.tenant_id
                   AND p.id = NEW.registered_by_principal_id AND p.kind = 'agent') THEN
        RAISE EXCEPTION 'account registrant must be an agent';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER agent_accounts_registrant_guard
    BEFORE INSERT OR UPDATE OF registered_by_principal_id ON agent_accounts
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_agent_account_registrant();

CREATE TABLE account_allowance_windows (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    unit text NOT NULL CHECK (unit IN ('requests', 'tokens', 'cost_micros')),
    allowance bigint NOT NULL CHECK (allowance > 0),
    used bigint NOT NULL DEFAULT 0 CHECK (used >= 0),
    reserved bigint NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    pace_model text NOT NULL DEFAULT 'steady' CHECK (pace_model IN ('steady', 'frontload', 'unrestricted')),
    burst_ratio numeric(5, 4) NOT NULL DEFAULT 0.1 CHECK (burst_ratio >= 0 AND burst_ratio <= 1),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, account_id) REFERENCES agent_accounts(tenant_id, id),
    CHECK (ends_at > starts_at),
    CHECK (used + reserved <= allowance)
);
CREATE INDEX account_allowance_active_idx ON account_allowance_windows
    (tenant_id, account_id, starts_at, ends_at);
ALTER TABLE account_allowance_windows ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_allowance_windows FORCE ROW LEVEL SECURITY;
CREATE POLICY account_allowance_windows_tenant ON account_allowance_windows
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE account_reservations (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL,
    window_id uuid NOT NULL,
    reserved_units bigint NOT NULL CHECK (reserved_units > 0),
    actual_units bigint CHECK (actual_units >= 0),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'settled', 'released')),
    created_at timestamptz NOT NULL DEFAULT now(),
    settled_at timestamptz,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, run_id, window_id),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id),
    FOREIGN KEY (tenant_id, window_id) REFERENCES account_allowance_windows(tenant_id, id),
    CHECK ((state = 'active') = (settled_at IS NULL)),
    CHECK (state <> 'settled' OR actual_units IS NOT NULL)
);
ALTER TABLE account_reservations ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_reservations FORCE ROW LEVEL SECURITY;
CREATE POLICY account_reservations_tenant ON account_reservations
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_account_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM agent_runs r JOIN account_allowance_windows w
          ON w.tenant_id = r.tenant_id AND w.account_id = r.account_id
        WHERE r.tenant_id = NEW.tenant_id AND r.id = NEW.run_id
          AND w.id = NEW.window_id
    ) THEN
        RAISE EXCEPTION 'reservation account does not match run account';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER account_reservations_account_guard BEFORE INSERT ON account_reservations
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_account_reservation();

ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_profile_fk
    FOREIGN KEY (tenant_id, model_profile_id) REFERENCES model_profiles(tenant_id, id);
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_account_fk
    FOREIGN KEY (tenant_id, account_id) REFERENCES agent_accounts(tenant_id, id);
