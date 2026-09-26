-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P2. Some events on a visible node describe another node: a relation
-- names both ends, and a classic relation import records the other ticket's
-- key and title. Such an event is visible only when every node it names is,
-- exactly like the relation row itself (0821). The production copy has 43
-- relations that cross projects.
--
-- A caller who sees only some projects reads only project work in the event
-- feed (nodes, comments, attachments, relations, imports, journey, intake,
-- requirements, releases, knowledge, views, tags, kinds, its own profile):
-- the history of agent sessions, inboxes, access changes, hours, quotes, CRM,
-- stage handoffs, work orders, runs and approvals stays with the workspace,
-- even when recorded on a project node, except the caller's own (so a
-- project member's harness writes can return their event). Unknown domains
-- are hidden.
--
-- The references are tested against uncorrelated sub-selects of the caller's
-- visible nodes, which Postgres hashes once per statement; a per-row function
-- call made a guest's project overview ten times slower on production data.
CREATE FUNCTION aeon_uuid_or_null(candidate text) RETURNS uuid
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN candidate ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                THEN candidate::uuid END
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
                WHEN type LIKE 'relation.%' THEN
                    (before->>'source_node_id' IS NULL OR aeon_uuid_or_null(before->>'source_node_id') IN (SELECT n.id FROM nodes n))
                    AND (before->>'target_node_id' IS NULL OR aeon_uuid_or_null(before->>'target_node_id') IN (SELECT n.id FROM nodes n))
                    AND (after->>'source_node_id' IS NULL OR aeon_uuid_or_null(after->>'source_node_id') IN (SELECT n.id FROM nodes n))
                    AND (after->>'target_node_id' IS NULL OR aeon_uuid_or_null(after->>'target_node_id') IN (SELECT n.id FROM nodes n))
                WHEN type = 'import.relation' THEN
                    -- The other end is known only by its classic key; an
                    -- unresolved or invisible key hides the event.
                    after->'record'->>'target_key' IN (SELECT n.key FROM nodes n)
                    OR after->'record'->>'target_key' IN (SELECT a.key FROM node_key_aliases a)
                WHEN type = 'import.release_membership' THEN
                    (after->>'project_node_id' IS NULL OR aeon_uuid_or_null(after->>'project_node_id') IN (SELECT n.id FROM nodes n))
                    AND (after->>'release_node_id' IS NULL OR aeon_uuid_or_null(after->>'release_node_id') IN (SELECT n.id FROM nodes n))
                    AND (after->>'ticket_node_id' IS NULL OR aeon_uuid_or_null(after->>'ticket_node_id') IN (SELECT n.id FROM nodes n))
                ELSE true
            END)
        OR (node_id IS NULL AND (cardinality((SELECT aeon_current_principals())::uuid[]) = 0
                                 OR actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[]))));
