-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P2. Some events on a visible node describe another node: a relation
-- names both ends, and a classic relation import records the other ticket's
-- key and title. Such an event is visible only when every node it names is,
-- exactly like the relation row itself (0821). The production copy has 43
-- relations that cross projects.
CREATE FUNCTION aeon_uuid_or_null(candidate text) RETURNS uuid
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN candidate ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                THEN candidate::uuid END
$$;

CREATE FUNCTION aeon_event_refs_visible(p_tenant uuid, p_type text, p_before jsonb, p_after jsonb) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN p_type LIKE 'relation.%' THEN NOT EXISTS (
            SELECT 1 FROM (VALUES (p_before->>'source_node_id'), (p_before->>'target_node_id'),
                                  (p_after->>'source_node_id'), (p_after->>'target_node_id')) AS ref(id)
            WHERE ref.id IS NOT NULL AND NOT EXISTS (
                SELECT 1 FROM nodes n WHERE n.tenant_id = p_tenant AND n.id = aeon_uuid_or_null(ref.id)))
        WHEN p_type = 'import.relation' THEN
            -- The other end is known only by its classic key; an unresolved or
            -- invisible key hides the event (fail closed).
            EXISTS (SELECT 1 FROM nodes n
                    WHERE n.tenant_id = p_tenant AND n.key = p_after->'record'->>'target_key')
            OR EXISTS (SELECT 1 FROM node_key_aliases a JOIN nodes n ON n.tenant_id = a.tenant_id AND n.id = a.node_id
                       WHERE a.tenant_id = p_tenant AND a.key = p_after->'record'->>'target_key')
        WHEN p_type = 'import.release_membership' THEN NOT EXISTS (
            SELECT 1 FROM (VALUES (p_after->>'project_node_id'), (p_after->>'release_node_id'),
                                  (p_after->>'ticket_node_id')) AS ref(id)
            WHERE ref.id IS NOT NULL AND NOT EXISTS (
                SELECT 1 FROM nodes n WHERE n.tenant_id = p_tenant AND n.id = aeon_uuid_or_null(ref.id)))
        ELSE true
    END
$$;

DROP POLICY events_project_visibility ON events;
CREATE POLICY events_project_visibility ON events AS RESTRICTIVE FOR SELECT
    USING ((SELECT aeon_visible_all())
        OR (node_id IS NOT NULL
            AND EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = events.tenant_id AND n.id = events.node_id)
            AND aeon_event_refs_visible(events.tenant_id, events.type, events.before, events.after))
        OR (node_id IS NULL AND (cardinality((SELECT aeon_current_principals())::uuid[]) = 0
                                 OR actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[]))));
