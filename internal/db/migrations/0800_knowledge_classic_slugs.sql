-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-138: the classic import kept a knowledge entry's slug and metadata only
-- in fields.classic, but the knowledge module reads fields.slug and
-- fields.metadata, so imported entries had no slug and could not be opened.
-- Apply the importer's current mapping to those nodes. The event is recorded
-- as an import update: it becomes the import baseline, so a later delta import
-- neither rewrites these nodes nor reports them as changed in Aeon.
-- The migration runner wraps this file in one transaction; each tenant is
-- scoped explicitly, as in 0531.
DO $$
DECLARE
    tenant uuid;
    actor uuid;
    prior_setting text := current_setting('aeon.tenant_id', true);
    old_node nodes%ROWTYPE;
    new_node nodes%ROWTYPE;
    extra jsonb;
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        FOR old_node IN
            SELECT n.* FROM nodes n
            JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
            WHERE n.tenant_id=tenant
              AND k.slug IN ('memory','runbook','guideline','external_system','related_project')
              AND coalesce(n.fields->>'slug','')=''
              AND jsonb_typeof(n.fields->'classic'->'slug')='string'
              AND n.fields->'classic'->>'slug'<>''
            ORDER BY n.id FOR UPDATE OF n
        LOOP
            extra := jsonb_build_object('slug', old_node.fields->'classic'->'slug');
            IF jsonb_typeof(old_node.fields->'classic'->'metadata')='object'
               AND old_node.fields->'classic'->'metadata'<>'{}'::jsonb
               AND NOT old_node.fields ? 'metadata' THEN
                extra := extra || jsonb_build_object('metadata', old_node.fields->'classic'->'metadata');
            END IF;
            -- The importer that wrote the node stays its last writer.
            SELECT e.actor_principal_id INTO actor FROM events e
            WHERE e.tenant_id=tenant AND e.node_id=old_node.id
              AND e.type IN ('import.node_created','import.node_updated')
            ORDER BY e.id DESC LIMIT 1;
            IF actor IS NULL THEN
                CONTINUE;
            END IF;
            -- A mapping repair is not an edit: keep updated_at so lists keep their order.
            UPDATE nodes SET fields=old_node.fields || extra
            WHERE tenant_id=tenant AND id=old_node.id RETURNING * INTO new_node;
            INSERT INTO events(tenant_id,actor_principal_id,node_id,type,before,after)
            VALUES(tenant,actor,old_node.id,'import.node_updated',to_jsonb(old_node),to_jsonb(new_node));
        END LOOP;
    END LOOP;
    PERFORM set_config('aeon.tenant_id',coalesce(prior_setting,''),true);
END;
$$;
