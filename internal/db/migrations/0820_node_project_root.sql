-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P2. Every node carries its project: the nearest ancestor-or-self
-- node of the "project" kind (the same project the web shows for a node).
-- A project node is its own project; a node outside every project (CRM
-- organisations, contacts, quotes, workspace knowledge) has NULL. Row-level
-- security (0821) reads this column, so it is kept by triggers and can never
-- be written directly: inserts, re-parenting and kind changes recompute it,
-- and a change cascades down the subtree, stopping at nested projects.
ALTER TABLE nodes ADD COLUMN project_id uuid;

-- The runner wraps this file in one transaction. FORCE RLS applies to the
-- table-owning app role in production, so the backfill runs per tenant with
-- the tenant and an explicit all-projects visibility, as in 0800 and 0811.
DO $$
DECLARE
    tenant uuid;
    prior_tenant text := current_setting('aeon.tenant_id', true);
    prior_visible text := current_setting('aeon.visible_projects', true);
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        PERFORM set_config('aeon.visible_projects', '*', true);
        WITH RECURSIVE tree(id, project_id) AS (
            SELECT n.id, CASE WHEN k.slug = 'project' THEN n.id END
            FROM nodes n JOIN node_kinds k ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
            WHERE n.tenant_id = tenant AND n.parent_id IS NULL
            UNION ALL
            SELECT c.id, CASE WHEN k.slug = 'project' THEN c.id ELSE t.project_id END
            FROM tree t
            JOIN nodes c ON c.tenant_id = tenant AND c.parent_id = t.id
            JOIN node_kinds k ON k.tenant_id = c.tenant_id AND k.id = c.kind_id
        )
        UPDATE nodes n SET project_id = tree.project_id
        FROM tree
        WHERE n.tenant_id = tenant AND n.id = tree.id
          AND n.project_id IS DISTINCT FROM tree.project_id;
    END LOOP;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_tenant, ''), true);
    PERFORM set_config('aeon.visible_projects', coalesce(prior_visible, ''), true);
END;
$$;

CREATE INDEX nodes_project_idx ON nodes(tenant_id, project_id, id) WHERE deleted_at IS NULL;

-- BEFORE triggers run in name order: this one runs before nodes_tree_guard,
-- which still rejects a missing, deleted or invisible parent.
CREATE FUNCTION aeon_node_project_root() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    kind_slug text;
    parent_project uuid;
BEGIN
    SELECT k.slug INTO kind_slug FROM node_kinds k
    WHERE k.tenant_id = NEW.tenant_id AND k.id = NEW.kind_id;
    IF kind_slug = 'project' THEN
        NEW.project_id := NEW.id;
    ELSIF NEW.parent_id IS NULL THEN
        NEW.project_id := NULL;
    ELSE
        SELECT p.project_id INTO parent_project FROM nodes p
        WHERE p.tenant_id = NEW.tenant_id AND p.id = NEW.parent_id;
        NEW.project_id := parent_project;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER nodes_project_root
BEFORE INSERT OR UPDATE OF parent_id, kind_id, project_id ON nodes
FOR EACH ROW EXECUTE FUNCTION aeon_node_project_root();

-- A column trigger would miss values changed by a BEFORE trigger, so the
-- cascade compares the old and new rows. Each child update recomputes its
-- own project from the already updated parent and cascades one level on.
CREATE FUNCTION aeon_node_project_cascade() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE nodes c SET project_id = NEW.project_id
    WHERE c.tenant_id = NEW.tenant_id AND c.parent_id = NEW.id
      AND c.project_id IS DISTINCT FROM NEW.project_id
      AND NOT EXISTS (SELECT 1 FROM node_kinds k
                      WHERE k.tenant_id = c.tenant_id AND k.id = c.kind_id AND k.slug = 'project');
    RETURN NULL;
END;
$$;
CREATE TRIGGER nodes_project_cascade
AFTER UPDATE ON nodes
FOR EACH ROW WHEN (OLD.project_id IS DISTINCT FROM NEW.project_id)
EXECUTE FUNCTION aeon_node_project_cascade();
