-- SPDX-License-Identifier: AGPL-3.0-only
-- Run with psql -v ON_ERROR_STOP=1 against a disposable migrated database.
-- All fixtures, grants, and the non-superuser test role are rolled back.
BEGIN;
CREATE ROLE aeon_r1_contract_test NOLOGIN;
GRANT USAGE ON SCHEMA public TO aeon_r1_contract_test;
GRANT ALL ON ALL TABLES IN SCHEMA public TO aeon_r1_contract_test;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO aeon_r1_contract_test;

DO $$
DECLARE
    checked integer;
    forced integer;
BEGIN
    SELECT count(*), count(*) FILTER (WHERE c.relrowsecurity AND c.relforcerowsecurity)
    INTO checked, forced
    FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
    WHERE ns.nspname = 'public' AND c.relname = ANY(ARRAY[
        'node_kinds', 'node_key_counters', 'nodes', 'node_relations',
        'event_counters', 'events', 'node_embeddings',
        'node_embedding_jobs', 'saved_views', 'import_jobs']);
    IF checked <> 10 OR forced <> 10 THEN
        RAISE EXCEPTION 'R1 tenant tables must all have FORCE RLS';
    END IF;
END $$;

SET LOCAL ROLE aeon_r1_contract_test;
INSERT INTO tenants(id, slug, name) VALUES
 ('eae00000-0000-0000-0000-000000000001', 'aeon-r1-contract-a', 'Contract A'),
 ('eae00000-0000-0000-0000-000000000002', 'aeon-r1-contract-b', 'Contract B');
SELECT set_config('aeon.tenant_id', 'eae00000-0000-0000-0000-000000000001', true);
INSERT INTO principals(id, tenant_id, kind, name) VALUES
 ('eae00000-0000-0000-0000-000000000011', 'eae00000-0000-0000-0000-000000000001', 'person', 'Contract actor');
DO $$ BEGIN
    IF (SELECT count(*) FROM node_kinds) <> 9 THEN
        RAISE EXCEPTION 'new tenant starter kinds missing';
    END IF;
END $$;
-- The migration backfill calls this same idempotent function for existing tenants.
DELETE FROM node_kinds WHERE slug = 'guideline';
SELECT aeon_seed_node_kinds('eae00000-0000-0000-0000-000000000001');
DO $$ BEGIN
    IF (SELECT count(*) FROM node_kinds) <> 9 THEN
        RAISE EXCEPTION 'starter kind backfill failed';
    END IF;
END $$;

INSERT INTO nodes(tenant_id, id, key, kind_id, title, body)
SELECT 'eae00000-0000-0000-0000-000000000001',
       'eae00000-0000-0000-0000-000000000021', 'PAI-123', id,
       'English search title', 'Markdown body'
FROM node_kinds WHERE slug = 'project';
INSERT INTO nodes(tenant_id, id, key, kind_id, title, parent_id)
SELECT 'eae00000-0000-0000-0000-000000000001',
       'eae00000-0000-0000-0000-000000000022',
       aeon_next_node_key('eae00000-0000-0000-0000-000000000001', 'PAI'),
       id, 'Deutscher Begriff', 'eae00000-0000-0000-0000-000000000021'
FROM node_kinds WHERE slug = 'ticket';
DO $$
DECLARE
    rejected boolean;
BEGIN
    IF (SELECT key FROM nodes WHERE id = 'eae00000-0000-0000-0000-000000000022') <> 'PAI-124' THEN
        RAISE EXCEPTION 'imported key did not advance counter';
    END IF;
    IF (SELECT count(*) FROM node_embedding_jobs) <> 2 THEN
        RAISE EXCEPTION 'embedding jobs were not queued';
    END IF;
    IF (SELECT count(*) FROM aeon_search_nodes('English search', NULL, NULL)) <> 1
       OR (SELECT count(*) FROM aeon_search_nodes('Deutscher Begriff', NULL, NULL)) <> 1 THEN
        RAISE EXCEPTION 'weighted bilingual lexical search failed';
    END IF;
    rejected := false;
    BEGIN
        UPDATE nodes SET parent_id = 'eae00000-0000-0000-0000-000000000022'
        WHERE id = 'eae00000-0000-0000-0000-000000000021';
    EXCEPTION WHEN raise_exception THEN rejected := true;
    END;
    IF NOT rejected THEN RAISE EXCEPTION 'tree cycle accepted'; END IF;
    rejected := false;
    BEGIN
        UPDATE nodes SET deleted_at = now()
        WHERE id = 'eae00000-0000-0000-0000-000000000021';
    EXCEPTION WHEN raise_exception THEN rejected := true;
    END;
    IF NOT rejected THEN RAISE EXCEPTION 'parent with live child was deleted'; END IF;
END $$;
UPDATE node_kinds SET allowed_child_kinds = ARRAY[]::text[] WHERE slug = 'project';
DO $$
DECLARE
    rejected boolean := false;
BEGIN
    BEGIN
        INSERT INTO nodes(tenant_id, key, kind_id, title, parent_id)
        SELECT 'eae00000-0000-0000-0000-000000000001', 'PAI-125', id,
               'Disallowed child', 'eae00000-0000-0000-0000-000000000021'
        FROM node_kinds WHERE slug = 'task';
    EXCEPTION WHEN raise_exception THEN rejected := true;
    END;
    IF NOT rejected THEN RAISE EXCEPTION 'disallowed child kind accepted'; END IF;
END $$;
-- Existing children can still be reordered, deleted, and restored after narrowing.
UPDATE nodes SET parent_id = parent_id WHERE id = 'eae00000-0000-0000-0000-000000000022';
UPDATE nodes SET deleted_at = now() WHERE id = 'eae00000-0000-0000-0000-000000000022';
UPDATE nodes SET deleted_at = NULL WHERE id = 'eae00000-0000-0000-0000-000000000022';

INSERT INTO events(tenant_id, actor_principal_id, node_id, type, after)
VALUES ('eae00000-0000-0000-0000-000000000001',
        'eae00000-0000-0000-0000-000000000011',
        'eae00000-0000-0000-0000-000000000021', 'node.created', '{}'::jsonb);
INSERT INTO events(tenant_id, actor_principal_id, node_id, type, before, after, undo_of)
VALUES ('eae00000-0000-0000-0000-000000000001',
        'eae00000-0000-0000-0000-000000000011',
        'eae00000-0000-0000-0000-000000000021', 'node.undone', '{}'::jsonb, '{}'::jsonb, 1);
DO $$
DECLARE
    rejected boolean := false;
BEGIN
    IF (SELECT max(id) FROM events) <> 2 THEN RAISE EXCEPTION 'tenant event IDs incorrect'; END IF;
    BEGIN
        INSERT INTO events(tenant_id, actor_principal_id, type, after, undo_of)
        VALUES ('eae00000-0000-0000-0000-000000000001',
                'eae00000-0000-0000-0000-000000000011', 'node.undone', '{}'::jsonb, 1);
    EXCEPTION WHEN unique_violation THEN rejected := true;
    END;
    IF NOT rejected THEN RAISE EXCEPTION 'duplicate undo accepted'; END IF;
END $$;
SET LOCAL ROLE aeon;
DO $$
DECLARE
    rejected boolean := false;
BEGIN
    BEGIN
        UPDATE events SET type = 'node.changed'
        WHERE tenant_id = 'eae00000-0000-0000-0000-000000000001' AND id = 1;
    EXCEPTION WHEN raise_exception THEN rejected := true;
    END;
    IF NOT rejected THEN RAISE EXCEPTION 'append-only event update accepted'; END IF;
END $$;
SET LOCAL ROLE aeon_r1_contract_test;

INSERT INTO node_embeddings(tenant_id, node_id, model, content_hash, embedding)
VALUES ('eae00000-0000-0000-0000-000000000001',
        'eae00000-0000-0000-0000-000000000021', 'test', repeat('a', 64),
        array_fill(0.1::real, ARRAY[1536])::halfvec(1536));
DO $$ BEGIN
    IF (SELECT count(*) FROM aeon_search_nodes('neverlexicalmatch',
        array_fill(0.1::real, ARRAY[1536])::halfvec(1536), 'test')) <> 0 THEN
        RAISE EXCEPTION 'stale embedding visible while job queued';
    END IF;
END $$;
DELETE FROM node_embedding_jobs WHERE node_id = 'eae00000-0000-0000-0000-000000000021';
DO $$ BEGIN
    IF (SELECT count(*) FROM aeon_search_nodes('neverlexicalmatch',
        array_fill(0.1::real, ARRAY[1536])::halfvec(1536), 'test')) <> 1 THEN
        RAISE EXCEPTION 'hybrid vector result missing';
    END IF;
END $$;

INSERT INTO saved_views(tenant_id, owner_principal_id, name)
VALUES ('eae00000-0000-0000-0000-000000000001',
        'eae00000-0000-0000-0000-000000000011', 'Saved list');
INSERT INTO import_jobs(tenant_id, created_by_principal_id, source)
VALUES ('eae00000-0000-0000-0000-000000000001',
        'eae00000-0000-0000-0000-000000000011', 'contract fixture');
SELECT set_config('aeon.tenant_id', 'eae00000-0000-0000-0000-000000000002', true);
DO $$ BEGIN
    IF (SELECT count(*) FROM node_kinds) <> 9 THEN RAISE EXCEPTION 'second tenant starter kinds missing'; END IF;
    IF (SELECT count(*) FROM nodes) <> 0 OR (SELECT count(*) FROM events) <> 0
       OR (SELECT count(*) FROM saved_views) <> 0 OR (SELECT count(*) FROM import_jobs) <> 0 THEN
        RAISE EXCEPTION 'cross-tenant row leaked';
    END IF;
END $$;
SELECT set_config('aeon.tenant_id', '', true);
DO $$ BEGIN
    IF (SELECT count(*) FROM nodes) <> 0 OR (SELECT count(*) FROM events) <> 0 THEN
        RAISE EXCEPTION 'unset tenant exposed rows';
    END IF;
END $$;
ROLLBACK;
