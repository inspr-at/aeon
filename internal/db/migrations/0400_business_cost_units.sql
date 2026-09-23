-- SPDX-License-Identifier: AGPL-3.0-only
-- Cost units are tenant-configured nodes. Rate rows are effective-dated inputs;
-- quotes and hours copy the selected amount into immutable financial snapshots.
CREATE FUNCTION aeon_business_node_kind(p_tenant uuid, p_node uuid, p_slug text)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1 FROM nodes n JOIN node_kinds k
          ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
        WHERE n.tenant_id = p_tenant AND n.id = p_node
          AND n.deleted_at IS NULL AND k.slug = p_slug
    );
$$;

CREATE TABLE cost_unit_rates (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    cost_unit_node_id uuid NOT NULL,
    unit text NOT NULL CHECK (unit IN ('hour', 'day', 'item')),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    internal_amount numeric(18, 4) NOT NULL CHECK (internal_amount >= 0),
    bill_amount numeric(18, 4) NOT NULL CHECK (bill_amount >= 0),
    effective_from date NOT NULL,
    effective_until date,
    created_by_principal_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, cost_unit_node_id, unit, currency, effective_from),
    FOREIGN KEY (tenant_id, cost_unit_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK (effective_until IS NULL OR effective_until > effective_from)
);
CREATE INDEX cost_unit_rates_lookup_idx ON cost_unit_rates
    (tenant_id, cost_unit_node_id, unit, currency, effective_from DESC);
ALTER TABLE cost_unit_rates ENABLE ROW LEVEL SECURITY;
ALTER TABLE cost_unit_rates FORCE ROW LEVEL SECURITY;
CREATE POLICY cost_unit_rates_tenant ON cost_unit_rates
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_cost_unit_rate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT aeon_business_node_kind(NEW.tenant_id, NEW.cost_unit_node_id, 'cost_unit') THEN
        RAISE EXCEPTION 'rate requires a live cost_unit node';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER cost_unit_rates_kind_guard BEFORE INSERT OR UPDATE OF cost_unit_node_id
    ON cost_unit_rates FOR EACH ROW EXECUTE FUNCTION aeon_guard_cost_unit_rate();
CREATE FUNCTION aeon_guard_cost_unit_rate_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'cost unit rates are historical';
    END IF;
    IF to_jsonb(NEW) - 'effective_until' <> to_jsonb(OLD) - 'effective_until'
       OR NEW.effective_until IS NULL
       OR (OLD.effective_until IS NOT NULL AND NEW.effective_until > OLD.effective_until) THEN
        RAISE EXCEPTION 'only closing a rate interval is allowed';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER cost_unit_rates_history_guard BEFORE UPDATE OR DELETE ON cost_unit_rates
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_cost_unit_rate_history();
