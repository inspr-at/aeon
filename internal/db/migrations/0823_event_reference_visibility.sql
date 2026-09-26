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
-- (events are append-only, so the list never changes). The before and after
-- snapshots are walked, objects and arrays, including fields: every string
-- that is a UUID is collected. A collected UUID is a node reference when it
-- is the id of a node of the same tenant, deleted nodes included. One query
-- against the nodes primary key decides the whole set. Principal, event,
-- role and binding ids are not nodes and do not count. Reference keys
-- (parent_id, project_id, node_id, feature_id, release_id, ticket_id,
-- requirement_id, *_node_id(s), *_ticket_ids) and a batch's items[].id still
-- mark a value that was detected as a reference but does not resolve: that
-- value, and a structure nested deeper than 16 levels, is the nil UUID,
-- which names no node, so the event stays hidden (fail closed). Those keys
-- are not how a UUID becomes a reference. Inside fields and the classic
-- record the same UUID rule applies, but a non-UUID there is not a broken
-- reference (classic ids are integers). A classic relation's target key is
-- resolved to its node when the event is written, with the writer's
-- visibility. Reading then never parses the snapshots: the policy tests
-- node_refs against the caller's visible nodes, a set Postgres hashes once
-- per statement.
--
-- Every arm of the policy but the every-project one applies that test: an
-- event with or without a node, and the caller's own events alike. Events
-- without a node are workspace activity: readable by their actor and by
-- explicit service paths (aeon.system), never by a transaction that has
-- neither a principal nor a service visibility.
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

-- candidates: UUID strings that count only when they are nodes.
-- required: UUID strings detected as references; a miss becomes the nil UUID.
-- markers: nil UUIDs for references that did not resolve, and for a structure
-- nested deeper than 16 levels. strict is false inside fields and record.
CREATE FUNCTION aeon_json_collect(snapshot jsonb, depth integer, strict boolean,
    OUT candidates uuid[], OUT required uuid[], OUT markers uuid[])
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    member record;
    element jsonb;
    sub_c uuid[];
    sub_r uuid[];
    sub_m uuid[];
    ref uuid;
    nil_id uuid := '00000000-0000-0000-0000-000000000000';
    loose boolean;
BEGIN
    candidates := '{}';
    required := '{}';
    markers := '{}';
    IF snapshot IS NULL OR jsonb_typeof(snapshot) = 'null' THEN
        RETURN;
    END IF;
    IF jsonb_typeof(snapshot) NOT IN ('object', 'array') THEN
        IF jsonb_typeof(snapshot) = 'string' THEN
            ref := aeon_uuid_or_null(snapshot #>> '{}');
            IF ref IS NOT NULL THEN
                candidates := ARRAY[ref];
            END IF;
        END IF;
        RETURN;
    END IF;
    IF depth > 16 THEN
        markers := ARRAY[nil_id];
        RETURN;
    END IF;
    IF jsonb_typeof(snapshot) = 'array' THEN
        FOR element IN SELECT value FROM jsonb_array_elements(snapshot) LOOP
            SELECT c.candidates, c.required, c.markers INTO sub_c, sub_r, sub_m
            FROM aeon_json_collect(element, depth + 1, strict) AS c;
            candidates := candidates || sub_c;
            required := required || sub_r;
            markers := markers || sub_m;
        END LOOP;
        RETURN;
    END IF;
    FOR member IN SELECT key, value FROM jsonb_each(snapshot) LOOP
        loose := member.key IN ('fields', 'record');
        IF strict AND NOT loose AND aeon_node_ref_key(member.key) THEN
            FOREACH ref IN ARRAY aeon_json_ref_values(member.value) LOOP
                IF ref = nil_id THEN
                    markers := markers || ref;
                ELSE
                    required := required || ref;
                END IF;
            END LOOP;
            CONTINUE;
        END IF;
        IF jsonb_typeof(member.value) IN ('object', 'array') THEN
            SELECT c.candidates, c.required, c.markers INTO sub_c, sub_r, sub_m
            FROM aeon_json_collect(member.value, depth + 1, strict AND NOT loose) AS c;
            candidates := candidates || sub_c;
            required := required || sub_r;
            markers := markers || sub_m;
        ELSIF jsonb_typeof(member.value) = 'string' THEN
            ref := aeon_uuid_or_null(member.value #>> '{}');
            IF ref IS NOT NULL THEN
                candidates := candidates || ref;
            END IF;
        END IF;
    END LOOP;
END;
$$;

-- The items a batch names by their own id.
CREATE FUNCTION aeon_json_item_ids(snapshot jsonb) RETURNS uuid[]
LANGUAGE sql IMMUTABLE AS $$
    SELECT coalesce(array_agg(coalesce(aeon_uuid_or_null(item->>'id'), '00000000-0000-0000-0000-000000000000'::uuid)), '{}'::uuid[])
    FROM jsonb_array_elements(CASE WHEN jsonb_typeof(snapshot->'items') = 'array' THEN snapshot->'items' ELSE '[]'::jsonb END) AS item
$$;

-- The node lookup sees every node of the tenant, deleted ones included, so a
-- writer who cannot see a named node still records it. The caller's
-- visibility is restored before the classic key lookup, which keeps the
-- writer's view: a target they cannot see becomes the nil UUID.
CREATE FUNCTION aeon_event_node_refs(p_tenant uuid, p_type text, p_before jsonb, p_after jsonb) RETURNS uuid[]
LANGUAGE plpgsql VOLATILE AS $$
DECLARE
    nil_id uuid := '00000000-0000-0000-0000-000000000000';
    prior text := current_setting('aeon.visible_projects', true);
    candidates uuid[] := '{}';
    required uuid[] := '{}';
    markers uuid[] := '{}';
    sub_c uuid[];
    sub_r uuid[];
    sub_m uuid[];
    ref uuid;
    found uuid[] := '{}';
    probe uuid[];
    target_key text;
    target uuid;
    extra uuid[] := '{}';
BEGIN
    SELECT c.candidates, c.required, c.markers INTO sub_c, sub_r, sub_m
    FROM aeon_json_collect(p_before, 0, true) AS c;
    candidates := sub_c;
    required := sub_r;
    markers := sub_m;
    SELECT c.candidates, c.required, c.markers INTO sub_c, sub_r, sub_m
    FROM aeon_json_collect(p_after, 0, true) AS c;
    candidates := candidates || sub_c;
    required := required || sub_r;
    markers := markers || sub_m;
    IF p_type = 'node.bulk_changed' THEN
        FOREACH ref IN ARRAY aeon_json_item_ids(p_before) || aeon_json_item_ids(p_after) LOOP
            IF ref = nil_id THEN
                markers := markers || ref;
            ELSE
                required := required || ref;
            END IF;
        END LOOP;
    END IF;
    probe := candidates || required;
    IF cardinality(probe) > 0 THEN
        -- Raised outside the lookup's subtransaction so the query sees every
        -- project, then put back even when the lookup fails.
        PERFORM set_config('aeon.visible_projects', '*', true);
        BEGIN
            SELECT coalesce(array_agg(DISTINCT n.id), '{}') INTO found
            FROM unnest(probe) AS c(id)
            JOIN nodes n ON n.tenant_id = p_tenant AND n.id = c.id;
        EXCEPTION WHEN OTHERS THEN
            PERFORM set_config('aeon.visible_projects', coalesce(prior, ''), true);
            RAISE;
        END;
        PERFORM set_config('aeon.visible_projects', coalesce(prior, ''), true);
    END IF;
    IF EXISTS (SELECT 1 FROM unnest(required) AS r(id)
               WHERE r.id <> nil_id AND NOT (r.id = ANY(found))) THEN
        markers := markers || nil_id;
    END IF;
    IF p_type = 'import.relation' THEN
        target_key := p_after->'record'->>'target_key';
        SELECT n.id INTO target FROM nodes n WHERE n.tenant_id = p_tenant AND n.key = target_key;
        IF target IS NULL THEN
            SELECT a.node_id INTO target FROM node_key_aliases a WHERE a.tenant_id = p_tenant AND a.key = target_key;
        END IF;
        extra := ARRAY[coalesce(target, nil_id)];
    END IF;
    RETURN found || markers || extra;
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
        OR (CASE
                WHEN node_id IS NULL THEN
                    (SELECT aeon_visibility_system())
                    OR actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[])
                ELSE EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = events.tenant_id AND n.id = events.node_id)
                    AND (split_part(type, '.', 1) IN ('node', 'nodes', 'comment', 'comments', 'attachment',
                            'attachments', 'relation', 'relations', 'import', 'journey', 'intake', 'requirement',
                            'requirements', 'release', 'releases', 'knowledge', 'view', 'views', 'tag', 'tags',
                            'kind', 'kinds', 'profile')
                         OR actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[]))
            END
            AND (cardinality(node_refs) = 0
                 OR NOT EXISTS (SELECT 1 FROM unnest(node_refs) AS ref(id)
                                WHERE ref.id NOT IN (SELECT n.id FROM nodes n)))));
