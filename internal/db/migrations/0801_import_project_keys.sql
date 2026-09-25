-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-143: imported projects kept their route key only in fields.classic.
-- The inbox routine reads fields.project_key when addressing a project.
-- Repair only missing values and advance the import baseline without moving
-- updated_at, so an unchanged delta import remains conflict-free.
DO $$
DECLARE
    tenant uuid;
    actor uuid;
    prior_setting text := current_setting('aeon.tenant_id', true);
    old_node nodes%ROWTYPE;
    new_node nodes%ROWTYPE;
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        FOR old_node IN
            SELECT n.* FROM nodes n
            JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
            WHERE n.tenant_id=tenant AND k.slug='project'
              AND coalesce(n.fields->>'project_key','')=''
              AND jsonb_typeof(n.fields->'classic'->'key')='string'
              AND n.fields->'classic'->>'key'<>''
            ORDER BY n.id FOR UPDATE OF n
        LOOP
            SELECT e.actor_principal_id INTO actor FROM events e
            WHERE e.tenant_id=tenant AND e.node_id=old_node.id
              AND e.type IN ('import.node_created','import.node_updated')
            ORDER BY e.id DESC LIMIT 1;
            IF actor IS NULL THEN
                CONTINUE;
            END IF;
            UPDATE nodes
            SET fields=old_node.fields || jsonb_build_object('project_key',old_node.fields->'classic'->'key')
            WHERE tenant_id=tenant AND id=old_node.id RETURNING * INTO new_node;
            INSERT INTO events(tenant_id,actor_principal_id,node_id,type,before,after)
            VALUES(tenant,actor,old_node.id,'import.node_updated',to_jsonb(old_node),to_jsonb(new_node));
        END LOOP;
    END LOOP;
    PERFORM set_config('aeon.tenant_id',coalesce(prior_setting,''),true);
END;
$$;
