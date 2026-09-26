-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P2. An event about a visible node can still describe other nodes:
-- a relation names both ends, a move names the former parent, a project move
-- names the former project and its journey feature and release, a classic
-- import snapshot names the parent and project, and a classic relation names
-- the other ticket by key. For a caller who sees only some projects, such an
-- event is visible only when every node it names is. The production copy has
-- 43 relations that cross projects, and moved tickets keep their history.
--
-- Each event records the nodes it names in node_refs when it is written
-- (events are append-only, so the list never changes): every value of a
-- node-reference key (parent_id, project_id, node_id, feature_id, release_id,
-- ticket_id, requirement_id, *_node_id(s), *_ticket_ids) at the top level of
-- its before and after snapshots and inside their nested objects (node,
-- journey, ...), except the free-form fields and the classic record. A value
-- that is not a node ID is recorded as the nil UUID, which names no node, so
-- the event stays hidden (fail closed). A classic relation's target key is
-- resolved to its node when the event is written. Reading then never parses
-- the snapshots: the policy tests node_refs against the caller's visible
-- nodes, a set Postgres hashes once per statement.
--
-- A caller who sees only some projects reads only project work in the event
-- feed (nodes, comments, attachments, relations, imports, journey, intake,
-- requirements, releases, knowledge, views, tags, kinds, its own profile):
-- the history of agent sessions, inboxes, access changes, hours, quotes, CRM,
-- stage handoffs, work orders, runs and approvals stays with the workspace,
-- even when recorded on a project node, except the caller's own (so a
-- project member's harness writes can return their event). Unknown domains
-- are hidden.
CREATE FUNCTION aeon_uuid_or_null(candidate text) RETURNS uuid
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN candidate ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                THEN candidate::uuid END
$$;

CREATE FUNCTION aeon_node_ref_key(k text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
    SELECT k IN ('parent_id', 'project_id', 'node_id', 'feature_id', 'release_id', 'ticket_id', 'requirement_id')
        OR k ~ '_node_ids?$' OR k ~ '_ticket_ids$'
$$;

-- The node IDs one JSON value holds: a string, or an array of strings; JSON
-- null holds none. Anything else is the nil UUID.
CREATE FUNCTION aeon_json_ref_values(v jsonb) RETURNS uuid[]
LANGUAGE sql IMMUTABLE AS $$
    SELECT coalesce(array_agg(coalesce(
               CASE WHEN jsonb_typeof(x) = 'string' THEN aeon_uuid_or_null(x #>> '{}') END,
               '00000000-0000-0000-0000-000000000000'::uuid)), '{}'::uuid[])
    FROM jsonb_array_elements(CASE jsonb_typeof(v)
                                  WHEN 'array' THEN v
                                  WHEN 'null' THEN '[]'::jsonb
                                  ELSE jsonb_build_array(v) END) AS x
    WHERE jsonb_typeof(x) <> 'null'
$$;

CREATE FUNCTION aeon_json_node_refs(snapshot jsonb) RETURNS uuid[]
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    refs uuid[] := '{}';
    top record;
    nested record;
BEGIN
    IF snapshot IS NULL OR jsonb_typeof(snapshot) <> 'object' THEN
        RETURN refs;
    END IF;
    FOR top IN SELECT key, value FROM jsonb_each(snapshot) LOOP
        IF aeon_node_ref_key(top.key) THEN
            refs := refs || aeon_json_ref_values(top.value);
        ELSIF jsonb_typeof(top.value) = 'object' AND top.key NOT IN ('fields', 'record') THEN
            FOR nested IN SELECT key, value FROM jsonb_each(top.value) LOOP
                IF aeon_node_ref_key(nested.key) THEN
                    refs := refs || aeon_json_ref_values(nested.value);
                END IF;
            END LOOP;
        END IF;
    END LOOP;
    RETURN refs;
END;
$$;

-- Runs with the writer's visibility: a classic relation target it cannot
-- see, or that is not imported, becomes the nil UUID.
CREATE FUNCTION aeon_event_node_refs(p_tenant uuid, p_type text, p_before jsonb, p_after jsonb) RETURNS uuid[]
LANGUAGE plpgsql STABLE AS $$
DECLARE
    refs uuid[] := aeon_json_node_refs(p_before) || aeon_json_node_refs(p_after);
    target_key text;
    target uuid;
BEGIN
    IF p_type = 'import.relation' THEN
        target_key := p_after->'record'->>'target_key';
        SELECT n.id INTO target FROM nodes n WHERE n.tenant_id = p_tenant AND n.key = target_key;
        IF target IS NULL THEN
            SELECT a.node_id INTO target FROM node_key_aliases a WHERE a.tenant_id = p_tenant AND a.key = target_key;
        END IF;
        refs := refs || coalesce(target, '00000000-0000-0000-0000-000000000000'::uuid);
    END IF;
    RETURN refs;
END;
$$;

ALTER TABLE events ADD COLUMN node_refs uuid[] NOT NULL DEFAULT '{}';

CREATE FUNCTION aeon_event_set_node_refs() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.node_refs := aeon_event_node_refs(NEW.tenant_id, NEW.type, NEW.before, NEW.after);
    RETURN NEW;
END;
$$;
CREATE TRIGGER events_node_refs
BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION aeon_event_set_node_refs();

-- Backfill per tenant with every project visible. Events are append-only and
-- have no UPDATE policy; both are lifted for this one statement per tenant.
DO $$
DECLARE
    tenant uuid;
    prior_tenant text := current_setting('aeon.tenant_id', true);
    prior_visible text := current_setting('aeon.visible_projects', true);
BEGIN
    ALTER TABLE events DISABLE TRIGGER events_no_update_delete;
    CREATE POLICY events_node_refs_backfill ON events FOR UPDATE
        USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        PERFORM set_config('aeon.visible_projects', '*', true);
        UPDATE events e SET node_refs = computed.refs
        FROM (SELECT id, aeon_event_node_refs(tenant_id, type, before, after) AS refs
              FROM events WHERE tenant_id = tenant) AS computed
        WHERE e.tenant_id = tenant AND e.id = computed.id AND cardinality(computed.refs) > 0;
    END LOOP;
    DROP POLICY events_node_refs_backfill ON events;
    ALTER TABLE events ENABLE TRIGGER events_no_update_delete;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_tenant, ''), true);
    PERFORM set_config('aeon.visible_projects', coalesce(prior_visible, ''), true);
END;
$$;

DROP POLICY events_project_visibility ON events;
CREATE POLICY events_project_visibility ON events AS RESTRICTIVE FOR SELECT
    USING ((SELECT aeon_visible_all())
        OR (node_id IS NOT NULL
            AND EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = events.tenant_id AND n.id = events.node_id)
            AND CASE
                WHEN split_part(type, '.', 1) NOT IN ('node', 'nodes', 'comment', 'comments', 'attachment',
                    'attachments', 'relation', 'relations', 'import', 'journey', 'intake', 'requirement',
                    'requirements', 'release', 'releases', 'knowledge', 'view', 'views', 'tag', 'tags',
                    'kind', 'kinds', 'profile') THEN actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[])
                ELSE cardinality(node_refs) = 0
                     OR NOT EXISTS (SELECT 1 FROM unnest(node_refs) AS ref(id)
                                    WHERE ref.id NOT IN (SELECT n.id FROM nodes n))
            END)
        OR (node_id IS NULL AND (cardinality((SELECT aeon_current_principals())::uuid[]) = 0
                                 OR actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[]))));
