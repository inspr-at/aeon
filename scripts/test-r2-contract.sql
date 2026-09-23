-- SPDX-License-Identifier: AGPL-3.0-only
-- Run with psql -X -v ON_ERROR_STOP=1 against a disposable migrated database.
-- The fixture, test role and all writes roll back.
BEGIN;
CREATE ROLE aeon_r2_contract_test NOLOGIN;
GRANT USAGE ON SCHEMA public TO aeon_r2_contract_test;
GRANT ALL ON ALL TABLES IN SCHEMA public TO aeon_r2_contract_test;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO aeon_r2_contract_test;
DO $$
DECLARE checked integer; forced integer;
BEGIN
    SELECT count(*), count(*) FILTER (WHERE c.relrowsecurity AND c.relforcerowsecurity)
      INTO checked, forced
    FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
    WHERE ns.nspname = 'public' AND c.relname = ANY(ARRAY[
        'inbox_messages', 'inbox_delivery_targets', 'inbox_wakes',
        'work_orders', 'work_criteria', 'agent_runs', 'run_telemetry',
        'work_evidence', 'approval_requests', 'approval_decisions',
        'agent_permission_grants', 'model_profiles', 'model_role_routes',
        'agent_accounts', 'account_allowance_windows', 'account_reservations']);
    IF checked <> 16 OR forced <> 16 THEN
        RAISE EXCEPTION 'all 16 R2 tables require FORCE RLS';
    END IF;
END $$;
SET LOCAL ROLE aeon_r2_contract_test;
INSERT INTO tenants(id, slug, name) VALUES
 ('eae20000-0000-0000-0000-000000000001', 'aeon-r2-contract-a', 'A'),
 ('eae20000-0000-0000-0000-000000000002', 'aeon-r2-contract-b', 'B');
SELECT set_config('aeon.tenant_id', 'eae20000-0000-0000-0000-000000000001', true);
DO $$ BEGIN
    IF (SELECT count(*) FROM node_kinds WHERE slug = 'work_order') <> 1 THEN
        RAISE EXCEPTION 'new tenant lacks work_order kind';
    END IF;
END $$;
INSERT INTO principals(id, tenant_id, kind, name) VALUES
 ('eae20000-0000-0000-0000-000000000011', 'eae20000-0000-0000-0000-000000000001', 'person', 'Owner'),
 ('eae20000-0000-0000-0000-000000000012', 'eae20000-0000-0000-0000-000000000001', 'agent', 'Worker');
INSERT INTO nodes(tenant_id, id, key, kind_id, title)
SELECT 'eae20000-0000-0000-0000-000000000001',
       'eae20000-0000-0000-0000-000000000021', 'WOR-1', id, 'Contract work'
FROM node_kinds WHERE slug = 'work_order';
INSERT INTO work_orders(tenant_id, node_id, requested_by_principal_id, assignee_principal_id,
                        max_cost_micros, max_duration_seconds)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000021',
        'eae20000-0000-0000-0000-000000000011',
        'eae20000-0000-0000-0000-000000000012', 1000000, 3600);
INSERT INTO work_criteria(tenant_id, id, work_order_id, position, description)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000022',
        'eae20000-0000-0000-0000-000000000021', 0, 'Check result');
INSERT INTO agent_runs(tenant_id, id, work_order_id, agent_principal_id)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000023',
        'eae20000-0000-0000-0000-000000000021',
        'eae20000-0000-0000-0000-000000000012');
INSERT INTO events(tenant_id, actor_principal_id, type, after)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000011', 'inbox.sent', '{}'::jsonb);
INSERT INTO inbox_messages(tenant_id, id, sender_principal_id, recipient_principal_id,
                           sent_event_id, body, idempotency_key)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000024',
        'eae20000-0000-0000-0000-000000000011',
        'eae20000-0000-0000-0000-000000000012', 1, 'Ready?', 'test-1');
INSERT INTO approval_requests(tenant_id, id, proposed_by_principal_id,
                              agent_principal_id, scope, resource_kind, rationale, expires_at)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000025',
        'eae20000-0000-0000-0000-000000000012',
        'eae20000-0000-0000-0000-000000000012',
        'work.execute', 'tenant', 'Run work', now() + interval '1 hour');
DO $$
DECLARE rejected boolean;
BEGIN
    rejected := false;
    BEGIN
        INSERT INTO approval_decisions(tenant_id, request_id, decided_by_principal_id, decision)
        VALUES ('eae20000-0000-0000-0000-000000000001',
                'eae20000-0000-0000-0000-000000000025',
                'eae20000-0000-0000-0000-000000000012', 'approved');
    EXCEPTION WHEN raise_exception THEN rejected := true;
    END;
    IF NOT rejected THEN RAISE EXCEPTION 'agent self-approval accepted'; END IF;
END $$;
INSERT INTO approval_decisions(tenant_id, request_id, decided_by_principal_id, decision)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000025',
        'eae20000-0000-0000-0000-000000000011', 'approved');
INSERT INTO agent_permission_grants(tenant_id, approval_request_id,
                                    agent_principal_id, scope, resource_kind, valid_until)
SELECT tenant_id, id, agent_principal_id, scope, resource_kind, expires_at
FROM approval_requests WHERE id = 'eae20000-0000-0000-0000-000000000025';
INSERT INTO agent_accounts(tenant_id, id, account_key, harness, daemon_id,
                           registered_by_principal_id, label)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000026', 'local-a', 'codex', 'daemon-a',
        'eae20000-0000-0000-0000-000000000012', 'Account A');
INSERT INTO account_allowance_windows(tenant_id, id, account_id, starts_at,
                                      ends_at, unit, allowance)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000027',
        'eae20000-0000-0000-0000-000000000026', now(), now() + interval '1 day',
        'requests', 10);
UPDATE agent_runs SET account_id = 'eae20000-0000-0000-0000-000000000026'
WHERE id = 'eae20000-0000-0000-0000-000000000023';
INSERT INTO account_reservations(tenant_id, run_id, window_id, reserved_units)
VALUES ('eae20000-0000-0000-0000-000000000001',
        'eae20000-0000-0000-0000-000000000023',
        'eae20000-0000-0000-0000-000000000027', 1);
SELECT set_config('aeon.tenant_id', 'eae20000-0000-0000-0000-000000000002', true);
DO $$ BEGIN
    IF (SELECT count(*) FROM inbox_messages) <> 0
       OR (SELECT count(*) FROM work_orders) <> 0
       OR (SELECT count(*) FROM agent_permission_grants) <> 0
       OR (SELECT count(*) FROM agent_accounts) <> 0 THEN
        RAISE EXCEPTION 'R2 tenant data leaked';
    END IF;
END $$;
ROLLBACK;
