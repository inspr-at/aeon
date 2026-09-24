-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-75: open-period corrections, sealed-period immutability retained.
DROP TRIGGER time_entries_immutable ON time_entries;
ALTER TABLE time_entries ADD COLUMN updated_at timestamptz;
-- Scope the backfill for FORCE RLS even when the migration role is not a
-- superuser. This adds concurrency metadata, not a billing correction.
DO $$
DECLARE tenant_row record; prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    FOR tenant_row IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant_row.id::text, true);
        UPDATE time_entries SET updated_at = created_at WHERE tenant_id = tenant_row.id;
    END LOOP;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
END;
$$;
ALTER TABLE time_entries ALTER COLUMN updated_at SET NOT NULL;
ALTER TABLE time_entries ALTER COLUMN updated_at SET DEFAULT clock_timestamp();

DROP TRIGGER time_entries_guard ON time_entries;
CREATE OR REPLACE FUNCTION aeon_guard_time_entry() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p time_periods%ROWTYPE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT * INTO p FROM time_periods
          WHERE tenant_id = OLD.tenant_id AND id = OLD.period_id FOR UPDATE;
        IF NOT FOUND OR p.state <> 'open' THEN
            RAISE EXCEPTION 'approved periods are immutable';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND
       (NEW.tenant_id, NEW.id, NEW.period_id, NEW.principal_id, NEW.source,
        NEW.agent_run_id, NEW.currency, NEW.created_at) IS DISTINCT FROM
       (OLD.tenant_id, OLD.id, OLD.period_id, OLD.principal_id, OLD.source,
        OLD.agent_run_id, OLD.currency, OLD.created_at) THEN
        RAISE EXCEPTION 'time entry identity and provenance are immutable';
    END IF;
    SELECT * INTO p FROM time_periods
      WHERE tenant_id = NEW.tenant_id AND id = NEW.period_id FOR UPDATE;
    IF NOT FOUND OR p.state <> 'open' THEN
        RAISE EXCEPTION 'approved periods are immutable';
    END IF;
    IF p.principal_id <> NEW.principal_id
       OR NEW.started_at < p.starts_at OR NEW.ended_at > p.ends_at
       OR NOT aeon_business_node_kind(NEW.tenant_id, NEW.cost_unit_node_id, 'cost_unit')
       OR NOT EXISTS (SELECT 1 FROM nodes WHERE tenant_id = NEW.tenant_id
                      AND id = NEW.node_id AND deleted_at IS NULL) THEN
        RAISE EXCEPTION 'invalid time entry period, node or cost unit';
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
CREATE TRIGGER time_entries_guard BEFORE INSERT OR UPDATE OR DELETE ON time_entries
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_time_entry();
