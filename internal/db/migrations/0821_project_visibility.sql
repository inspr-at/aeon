-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P2. Project visibility in the data layer, fail closed.
--
-- db.InTenant sets aeon.visible_projects once per transaction:
--   '*'            every project and every workspace-level node (a workspace
--                  binding whose role holds nodes.read, or an explicit
--                  service path such as the importer or embedding worker);
--   '{uuid,...}'   only these projects (project bindings, e.g. Guest);
--   '' or unset    nothing project-scoped at all.
-- aeon.principal_ids names the caller (and its canonical person) so a
-- project-only caller still reads the events it wrote itself. aeon.system is
-- 'on' only for explicit service paths (db.AllProjects, NoProjects,
-- OnlyProjects), which may read workspace activity; a transaction with no
-- principal and no service visibility reads nothing (0823).
--
-- Each covered table gets one RESTRICTIVE policy next to its tenant policy.
-- Policies read the setting through scalar sub-selects, which Postgres runs
-- once per statement (an InitPlan), never once per row.
CREATE FUNCTION aeon_visible_all() RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT coalesce(current_setting('aeon.visible_projects', true), '') = '*'
$$;

CREATE FUNCTION aeon_visible_projects() RETURNS uuid[]
LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN coalesce(current_setting('aeon.visible_projects', true), '') IN ('', '*')
                THEN '{}'::uuid[]
                ELSE current_setting('aeon.visible_projects', true)::uuid[] END
$$;

CREATE FUNCTION aeon_visibility_system() RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT coalesce(current_setting('aeon.system', true), '') = 'on'
$$;

CREATE FUNCTION aeon_current_principals() RETURNS uuid[]
LANGUAGE sql STABLE AS $$
    SELECT coalesce(NULLIF(current_setting('aeon.principal_ids', true), '')::uuid[], '{}'::uuid[])
$$;

-- Keep in step with the Go registry (authz tests compare both). A workspace
-- binding shows every project when its role holds nodes.read; Guest is a
-- project-only role and never opens the workspace. A project binding shows
-- its project when the role holds nodes.read.
CREATE FUNCTION aeon_role_reads_nodes(p_tenant uuid, p_role uuid, p_scope text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1 FROM roles r
        WHERE r.tenant_id = p_tenant AND r.id = p_role
          AND CASE WHEN r.builtin THEN
                   r.key = ANY (CASE p_scope WHEN 'workspace' THEN ARRAY['owner','admin','member','viewer']
                                             ELSE ARRAY['owner','admin','member','viewer','guest'] END)
              ELSE EXISTS (SELECT 1 FROM role_permissions rp
                           WHERE rp.tenant_id = r.tenant_id AND rp.role_id = r.id
                             AND rp.permission = 'nodes.read') END)
$$;

-- The caller's visibility. Deactivated principals and principals whose
-- canonical person is deactivated see nothing; linked classic aliases act
-- through their canonical person's bindings.
CREATE FUNCTION aeon_principal_visibility(p_tenant uuid, p_principal uuid) RETURNS text
LANGUAGE plpgsql STABLE AS $$
DECLARE
    canonical uuid;
    active boolean;
    projects uuid[];
BEGIN
    SELECT c.id, p.status = 'active' AND c.status = 'active' INTO canonical, active
    FROM principals p
    JOIN principals c ON c.tenant_id = p.tenant_id AND c.id = coalesce(p.linked_to, p.id)
    WHERE p.tenant_id = p_tenant AND p.id = p_principal;
    IF canonical IS NULL OR NOT active THEN
        RETURN '';
    END IF;
    IF EXISTS (SELECT 1 FROM role_bindings b
               WHERE b.tenant_id = p_tenant AND b.principal_id = canonical
                 AND b.scope_type = 'workspace'
                 AND aeon_role_reads_nodes(p_tenant, b.role_id, 'workspace')) THEN
        RETURN '*';
    END IF;
    SELECT array_agg(DISTINCT b.scope_id ORDER BY b.scope_id) INTO projects
    FROM role_bindings b
    WHERE b.tenant_id = p_tenant AND b.principal_id = canonical
      AND b.scope_type = 'project'
      AND aeon_role_reads_nodes(p_tenant, b.role_id, 'project');
    RETURN coalesce(projects, '{}'::uuid[])::text;
END;
$$;

CREATE FUNCTION aeon_visibility_intersect(a text, b text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE
        WHEN coalesce(a, '') = '' OR coalesce(b, '') = '' THEN ''
        WHEN a = '*' THEN b
        WHEN b = '*' THEN a
        ELSE coalesce((SELECT array_agg(x ORDER BY x) FROM unnest(a::uuid[]) x WHERE x = ANY (b::uuid[])),
                      '{}'::uuid[])::text
    END
$$;

-- db.InTenant enters a principal's transaction with one call: tenant first
-- (the binding tables are tenant scoped), then the visibility. An agent key
-- is capped by its creator's visibility, as its permissions are.
CREATE FUNCTION aeon_enter_principal(p_tenant uuid, p_principal uuid, p_key_creator uuid) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE
    visibility text;
    canonical uuid;
BEGIN
    PERFORM set_config('aeon.tenant_id', p_tenant::text, true);
    PERFORM set_config('aeon.visible_projects', '', true);
    PERFORM set_config('aeon.principal_ids', '', true);
    PERFORM set_config('aeon.system', '', true);
    visibility := aeon_principal_visibility(p_tenant, p_principal);
    IF p_key_creator IS NOT NULL THEN
        visibility := aeon_visibility_intersect(visibility, aeon_principal_visibility(p_tenant, p_key_creator));
    END IF;
    SELECT coalesce(p.linked_to, p.id) INTO canonical FROM principals p
    WHERE p.tenant_id = p_tenant AND p.id = p_principal;
    PERFORM set_config('aeon.principal_ids',
        ARRAY[p_principal, coalesce(canonical, p_principal)]::text, true);
    PERFORM set_config('aeon.visible_projects', visibility, true);
    RETURN visibility;
END;
$$;

-- Rows that carry their project directly.
CREATE POLICY nodes_project_visibility ON nodes AS RESTRICTIVE
    USING ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]))
    WITH CHECK ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]));
-- Views pinned to a project follow it; workspace-wide views are workspace rows.
CREATE POLICY saved_views_project_visibility ON saved_views AS RESTRICTIVE
    USING ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]))
    WITH CHECK ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]));
CREATE POLICY project_group_members_project_visibility ON project_group_members AS RESTRICTIVE
    USING ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]))
    WITH CHECK ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]));

DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['journey_projects','journey_releases','journey_requirements',
        'journey_features','journey_tickets','journey_gates','journey_action_receipts',
        'intake_sources','intake_transcript_turns','intake_drafts','intake_citations',
        'intake_draft_ticket_suggestions','intake_draft_acceptances','stage_handoffs',
        'stage_handoff_classic_batch_aliases','crm_project_cooperation'] LOOP
        EXECUTE format('CREATE POLICY %I ON %I AS RESTRICTIVE
            USING ((SELECT aeon_visible_all()) OR project_node_id = ANY ((SELECT aeon_visible_projects())::uuid[]))
            WITH CHECK ((SELECT aeon_visible_all()) OR project_node_id = ANY ((SELECT aeon_visible_projects())::uuid[]))',
            t || '_project_visibility', t);
    END LOOP;
END;
$$;

CREATE POLICY harness_sessions_project_visibility ON harness_sessions AS RESTRICTIVE
    USING ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]))
    WITH CHECK ((SELECT aeon_visible_all()) OR project_id = ANY ((SELECT aeon_visible_projects())::uuid[]));

-- Rows that belong to a node follow the node: the nodes policy above applies
-- inside these sub-selects too, so a node in an invisible project hides them.
DO $$
DECLARE
    item text[];
BEGIN
    FOREACH item SLICE 1 IN ARRAY ARRAY[
        ARRAY['attachments','node_id'], ARRAY['node_embeddings','node_id'],
        ARRAY['node_embedding_jobs','node_id'], ARRAY['node_key_aliases','node_id'],
        ARRAY['work_orders','node_id'], ARRAY['time_entries','node_id']] LOOP
        EXECUTE format('CREATE POLICY %I ON %I AS RESTRICTIVE
            USING ((SELECT aeon_visible_all()) OR EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = %I.tenant_id AND n.id = %I.%I))
            WITH CHECK ((SELECT aeon_visible_all()) OR EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = %I.tenant_id AND n.id = %I.%I))',
            item[1] || '_project_visibility', item[1],
            item[1], item[1], item[2], item[1], item[1], item[2]);
    END LOOP;
END;
$$;

-- A relation is visible only when both ends are.
CREATE POLICY node_relations_project_visibility ON node_relations AS RESTRICTIVE
    USING ((SELECT aeon_visible_all()) OR (
        EXISTS (SELECT 1 FROM nodes s WHERE s.tenant_id = node_relations.tenant_id AND s.id = node_relations.source_node_id)
        AND EXISTS (SELECT 1 FROM nodes d WHERE d.tenant_id = node_relations.tenant_id AND d.id = node_relations.target_node_id)))
    WITH CHECK ((SELECT aeon_visible_all()) OR (
        EXISTS (SELECT 1 FROM nodes s WHERE s.tenant_id = node_relations.tenant_id AND s.id = node_relations.source_node_id)
        AND EXISTS (SELECT 1 FROM nodes d WHERE d.tenant_id = node_relations.tenant_id AND d.id = node_relations.target_node_id)));

-- Events about a node (comments, history, imports) follow the node. Events
-- without a node are workspace activity, not project data: a principal who
-- does not see the whole workspace reads only its own (a profile edit, a
-- sign-in); system code without a principal (sign-in itself) reads them.
CREATE POLICY events_project_visibility ON events AS RESTRICTIVE FOR SELECT
    USING ((SELECT aeon_visible_all())
        OR (node_id IS NOT NULL AND EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = events.tenant_id AND n.id = events.node_id))
        OR (node_id IS NULL AND (cardinality((SELECT aeon_current_principals())::uuid[]) = 0
                                 OR actor_principal_id = ANY ((SELECT aeon_current_principals())::uuid[]))));
CREATE POLICY events_project_insert ON events AS RESTRICTIVE FOR INSERT
    WITH CHECK ((SELECT aeon_visible_all()) OR node_id IS NULL
        OR EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id = events.tenant_id AND n.id = events.node_id));
