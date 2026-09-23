-- SPDX-License-Identifier: AGPL-3.0-only
-- The migration runner wraps this entire file in one transaction. Each tenant
-- is explicitly scoped just as db.InTenant scopes application transactions.
-- Preserve the raw classic snapshot; audit full node snapshots for undo/activity.
DO $$
DECLARE
    tenant uuid;
    actor uuid;
    prior_setting text := current_setting('aeon.tenant_id', true);
    old_node nodes%ROWTYPE;
    new_node nodes%ROWTYPE;
    canonical text;
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        actor := NULL;
        FOR old_node IN SELECT * FROM nodes WHERE tenant_id=tenant AND (
            (lower(btrim(state)) IN ('in-progress','in progress','inprogress','canceled')) OR
            (lower(btrim(state)) IN ('in_progress','cancelled','new','backlog','qa','done','delivered','accepted','archived','open','active','frozen','deleted') AND state<>lower(btrim(state)))
        ) ORDER BY id FOR UPDATE LOOP
            IF actor IS NULL THEN
                SELECT id INTO actor FROM principals
                WHERE tenant_id=tenant AND kind='agent' AND name='System' AND roles @> ARRAY['system']
                ORDER BY created_at,id LIMIT 1;
                IF actor IS NULL THEN
                    INSERT INTO principals(tenant_id,kind,name,roles)
                    VALUES(tenant,'agent','System',ARRAY['system']) RETURNING id INTO actor;
                    INSERT INTO events(tenant_id,actor_principal_id,type,after)
                    VALUES(tenant,actor,'principal.created',jsonb_build_object('id',actor,'kind','agent','name','System','roles',ARRAY['system']));
                END IF;
            END IF;
            canonical := CASE lower(btrim(old_node.state))
                WHEN 'in-progress' THEN 'in_progress'
                WHEN 'in progress' THEN 'in_progress'
                WHEN 'inprogress' THEN 'in_progress'
                WHEN 'canceled' THEN 'cancelled'
                ELSE lower(btrim(old_node.state)) END;
            -- A spelling normalisation is not a change anyone made: keep updated_at
            -- so lists keep their order, and use a type the activity timeline skips.
            UPDATE nodes SET state=canonical
            WHERE tenant_id=tenant AND id=old_node.id RETURNING * INTO new_node;
            INSERT INTO events(tenant_id,actor_principal_id,node_id,type,before,after)
            VALUES(tenant,actor,old_node.id,'node.state_normalized',to_jsonb(old_node),to_jsonb(new_node));
        END LOOP;
    END LOOP;
    PERFORM set_config('aeon.tenant_id',coalesce(prior_setting,''),true);
END;
$$;

-- Bound retained-user lookups to the small user-event subset of the log.
CREATE INDEX IF NOT EXISTS events_import_user_principal_idx
ON events(tenant_id, (after->'principal'->>'id'), id DESC)
WHERE type IN ('import.user_created','import.user_updated');
