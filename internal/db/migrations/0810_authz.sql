-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P1. Built-in role permissions live in the versioned Go registry.
ALTER TABLE principals ADD COLUMN status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'deactivated'));
ALTER TABLE agent_keys ADD COLUMN created_by_principal_id uuid;
ALTER TABLE agent_keys ADD CONSTRAINT agent_keys_creator_tenant
    FOREIGN KEY (tenant_id,created_by_principal_id) REFERENCES principals(tenant_id,id);

CREATE TABLE roles (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    builtin boolean NOT NULL DEFAULT false,
    based_on uuid,
    PRIMARY KEY (tenant_id,id),
    UNIQUE (tenant_id,key),
    FOREIGN KEY (tenant_id,based_on) REFERENCES roles(tenant_id,id),
    CHECK (key ~ '^[a-z][a-z0-9_]*$')
);
CREATE TABLE role_permissions (
    tenant_id uuid NOT NULL,
    role_id uuid NOT NULL,
    permission text NOT NULL,
    PRIMARY KEY (tenant_id,role_id,permission),
    FOREIGN KEY (tenant_id,role_id) REFERENCES roles(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE role_bindings (
    tenant_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL,
    role_id uuid NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('workspace','project')),
    scope_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,id),
    FOREIGN KEY (tenant_id,principal_id) REFERENCES principals(tenant_id,id),
    FOREIGN KEY (tenant_id,role_id) REFERENCES roles(tenant_id,id),
    FOREIGN KEY (tenant_id,scope_id) REFERENCES nodes(tenant_id,id),
    CHECK ((scope_type='workspace' AND scope_id IS NULL) OR
           (scope_type='project' AND scope_id IS NOT NULL))
);
CREATE UNIQUE INDEX role_bindings_workspace ON role_bindings(tenant_id,principal_id)
    WHERE scope_type='workspace';
CREATE UNIQUE INDEX role_bindings_project ON role_bindings(tenant_id,principal_id,scope_id)
    WHERE scope_type='project';

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE ROW LEVEL SECURITY;
CREATE POLICY roles_tenant ON roles USING (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
-- Tenant bootstrap inserts the tenant under the lookup sentinel. The trigger
-- may seed only fixed built-ins while that transaction still uses the sentinel.
CREATE POLICY roles_bootstrap_seed ON roles FOR INSERT WITH CHECK (
    current_setting('aeon.tenant_id',true)='00000000-0000-0000-0000-000000000000'
    AND builtin AND key IN ('owner','admin','member','viewer','guest','customer'));
ALTER TABLE role_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_permissions FORCE ROW LEVEL SECURITY;
CREATE POLICY role_permissions_tenant ON role_permissions USING (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
ALTER TABLE role_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY role_bindings_tenant ON role_bindings USING (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE FUNCTION aeon_builtin_role_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.builtin THEN RAISE EXCEPTION 'built-in roles are immutable' USING ERRCODE='23514'; END IF;
    RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END;
$$;
CREATE TRIGGER roles_builtin_immutable BEFORE UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION aeon_builtin_role_immutable();
CREATE FUNCTION aeon_builtin_role_permissions_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_id uuid;
DECLARE target_tenant uuid;
BEGIN
    target_id := CASE WHEN TG_OP='DELETE' THEN OLD.role_id ELSE NEW.role_id END;
    target_tenant := CASE WHEN TG_OP='DELETE' THEN OLD.tenant_id ELSE NEW.tenant_id END;
    IF EXISTS (SELECT 1 FROM roles WHERE tenant_id=target_tenant AND id=target_id AND builtin) THEN
        RAISE EXCEPTION 'built-in role permissions are immutable' USING ERRCODE='23514';
    END IF;
    RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END;
$$;
CREATE TRIGGER role_permissions_builtin_immutable BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION aeon_builtin_role_permissions_immutable();

CREATE FUNCTION aeon_seed_builtin_roles(target uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=public,pg_temp AS $$
BEGIN
    INSERT INTO roles(tenant_id,key,name,description,builtin)
    SELECT target, key, initcap(key), description, true FROM (VALUES
        ('owner','All workspace permissions'),
        ('admin','Workspace administration except ownership transfer'),
        ('member','Product work in accessible projects'),
        ('viewer','Read-only workspace access'),
        ('guest','Project reading and comments'),
        ('customer','Own quote portal')
    ) AS builtins(key,description)
    ON CONFLICT (tenant_id,key) DO NOTHING;
END;
$$;
REVOKE EXECUTE ON FUNCTION aeon_seed_builtin_roles(uuid) FROM PUBLIC;
CREATE FUNCTION aeon_seed_builtin_roles_trigger() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=public,pg_temp AS $$
BEGIN
    PERFORM aeon_seed_builtin_roles(NEW.id);
    RETURN NEW;
END;
$$;
REVOKE EXECUTE ON FUNCTION aeon_seed_builtin_roles_trigger() FROM PUBLIC;
CREATE TRIGGER tenants_seed_builtin_roles AFTER INSERT ON tenants
    FOR EACH ROW EXECUTE FUNCTION aeon_seed_builtin_roles_trigger();

-- Migration events belong to System, never to the person or agent receiving
-- the binding. Create it only in tenants where a binding is actually written.
CREATE FUNCTION aeon_authz_system_actor(target_tenant uuid) RETURNS uuid
LANGUAGE plpgsql AS $$
DECLARE actor uuid;
BEGIN
    SELECT id INTO actor FROM principals
    WHERE tenant_id=target_tenant AND kind='agent' AND name='System' AND roles @> ARRAY['system']
    ORDER BY created_at,id LIMIT 1;
    IF actor IS NULL THEN
        INSERT INTO principals(tenant_id,kind,name,roles)
        VALUES(target_tenant,'agent','System',ARRAY['system']) RETURNING id INTO actor;
        INSERT INTO events(tenant_id,actor_principal_id,type,after)
        VALUES(target_tenant,actor,'principal.created',
               jsonb_build_object('id',actor,'kind','agent','name','System','roles',ARRAY['system']));
    END IF;
    RETURN actor;
END;
$$;

-- A linked classic identity is an alias. The signed-in, unlinked person gets
-- the strongest classic role among their own labels and all linked aliases.
-- Subsequent imports never replace an existing workspace binding.
CREATE FUNCTION aeon_bind_legacy_principal(target_tenant uuid,target_principal uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE chosen_role uuid;
DECLARE new_binding uuid;
DECLARE canonical_id uuid;
BEGIN
    SELECT coalesce(linked_to,id) INTO canonical_id FROM principals
    WHERE tenant_id=target_tenant AND id=target_principal AND kind='person';
    IF canonical_id IS NULL THEN RETURN; END IF;
    SELECT r.id INTO chosen_role FROM (
      SELECT max(CASE
        WHEN 'super_admin'=ANY(p.roles) THEN 5
        WHEN 'admin'=ANY(p.roles) THEN 4
        WHEN 'member'=ANY(p.roles) OR 'reviewer'=ANY(p.roles) THEN 3
        WHEN 'external'=ANY(p.roles) THEN 2
        WHEN 'customer'=ANY(p.roles) THEN 1
        ELSE 0 END) AS rank
      FROM principals p WHERE p.tenant_id=target_tenant
        AND (p.id=canonical_id OR p.linked_to=canonical_id)
    ) mapped JOIN roles r ON r.tenant_id=target_tenant AND r.key=CASE mapped.rank
      WHEN 5 THEN 'owner' WHEN 4 THEN 'admin' WHEN 3 THEN 'member'
      WHEN 2 THEN 'guest' WHEN 1 THEN 'customer' END;
    IF chosen_role IS NULL THEN RETURN; END IF;
    INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type)
      VALUES(target_tenant,canonical_id,chosen_role,'workspace')
      ON CONFLICT DO NOTHING RETURNING id INTO new_binding;
    IF new_binding IS NOT NULL THEN
      INSERT INTO events(tenant_id,actor_principal_id,type,after)
      VALUES(target_tenant,aeon_authz_system_actor(target_tenant),'authz.binding_migrated',
        jsonb_build_object('principal_id',canonical_id,'role_id',chosen_role,'scope_type','workspace'));
    END IF;
END;
$$;

-- Keep this list in step with the versioned Go registry. Old key scopes may
-- contain unknown values; they are recorded, never silently granted.
CREATE FUNCTION aeon_authz_registry_permission(candidate text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
    SELECT EXISTS (
      SELECT 1 FROM (VALUES
        ('nodes','read write delete move restore configure'),
        ('kinds','read manage'), ('tags','read write manage'),
        ('relations','read write delete'), ('comments','read write delete'),
        ('attachments','read write delete'), ('knowledge','read write delete'),
        ('journey','read act manage'), ('requirements','read write agree'),
        ('releases','read write deploy'), ('intake','read write decide'),
        ('stage_handoffs','read write decide'), ('harness','read write worker control manage'),
        ('work_orders','read write assign'), ('runs','read write control claim'),
        ('run','create read claim telemetry'), ('account','read manage route probe'),
        ('approvals','read request propose decide decide_high revoke'), ('inbox','read send manage'),
        ('stage','prepare deploy verify apply'), ('models','read manage resolve'),
        ('plugins','read manage invoke'), ('imports','read manage'),
        ('views','read write share'), ('events','read undo undo_other'),
        ('search','read'), ('hours','read write approve'),
        ('quotes','read write issue accept delete manage portal_read portal_accept'),
        ('crm','read write manage'), ('cost_units','read write manage'),
        ('project_groups','read write'), ('profile','read write manage portal_read portal_write'),
        ('settings','read manage'), ('members','read manage'),
        ('roles','read manage'), ('keys','read manage'),
        ('audit','read'), ('authz','read'), ('ownership','transfer')
      ) AS registry(resource,actions)
      CROSS JOIN LATERAL unnest(string_to_array(registry.actions,' ')) AS a(action)
      WHERE candidate=registry.resource || '.' || a.action
    );
$$;

CREATE FUNCTION aeon_migrate_agent_binding(target_tenant uuid,target_agent uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE role_id uuid;
DECLARE new_binding uuid;
DECLARE mapped text[];
DECLARE unmapped text[];
BEGIN
    IF NOT EXISTS (SELECT 1 FROM principals p WHERE p.tenant_id=target_tenant
       AND p.id=target_agent AND p.kind='agent'
       AND NOT (p.roles && ARRAY['system','importer','operator','embedding',
           'quote_public_service','quote_confirmation_service']::text[])
       AND EXISTS (SELECT 1 FROM agent_keys k WHERE k.tenant_id=target_tenant
           AND k.principal_id=p.id AND k.revoked_at IS NULL
           AND (k.expires_at IS NULL OR k.expires_at>now()))) THEN RETURN; END IF;
    SELECT array_agg(DISTINCT permission ORDER BY permission)
             FILTER (WHERE aeon_authz_registry_permission(permission)),
           array_agg(DISTINCT scope ORDER BY scope)
             FILTER (WHERE NOT aeon_authz_registry_permission(permission))
      INTO mapped,unmapped
      FROM agent_keys k CROSS JOIN LATERAL unnest(k.scopes) AS s(scope)
      CROSS JOIN LATERAL (SELECT replace(s.scope,':','.') AS permission) normalized
      WHERE k.tenant_id=target_tenant AND k.principal_id=target_agent
        AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now());
    INSERT INTO roles(tenant_id,key,name,description)
      SELECT p.tenant_id,'agent_' || replace(p.id::text,'-',''),'Agent ' || p.name,
        'Permissions assigned to this agent'
      FROM principals p WHERE p.tenant_id=target_tenant AND p.id=target_agent
      ON CONFLICT (tenant_id,key) DO UPDATE SET name=EXCLUDED.name
      RETURNING id INTO role_id;
    INSERT INTO role_permissions(tenant_id,role_id,permission)
      SELECT target_tenant,role_id,p.permission
      FROM unnest(coalesce(mapped,'{}'::text[])) AS p(permission)
      ON CONFLICT DO NOTHING;
    INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type)
      VALUES(target_tenant,target_agent,role_id,'workspace')
      ON CONFLICT DO NOTHING RETURNING id INTO new_binding;
    IF new_binding IS NOT NULL THEN
      INSERT INTO events(tenant_id,actor_principal_id,type,after)
      VALUES(target_tenant,aeon_authz_system_actor(target_tenant),'authz.agent_binding_migrated',
        jsonb_build_object('principal_id',target_agent,'role_id',role_id,
          'scope_type','workspace','permissions',coalesce(mapped,'{}'::text[]),
          'unmapped_scopes',coalesce(unmapped,'{}'::text[])));
    END IF;
END;
$$;

-- The runner wraps this file in one transaction. FORCE RLS applies to its
-- table-owning app role, so every data step must run in tenant context.
DO $$
DECLARE
    tenant uuid;
    person_id uuid;
    agent_id uuid;
    fallback_id uuid;
    owner_role uuid;
    prior_setting text := current_setting('aeon.tenant_id',true);
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id',tenant::text,true);
        PERFORM aeon_seed_builtin_roles(tenant);
        FOR person_id IN SELECT id FROM principals
          WHERE tenant_id=tenant AND kind='person' AND linked_to IS NULL ORDER BY created_at,id LOOP
            PERFORM aeon_bind_legacy_principal(tenant,person_id);
        END LOOP;
        FOR agent_id IN SELECT p.id FROM principals p WHERE p.tenant_id=tenant
          AND p.kind='agent' AND NOT (p.roles && ARRAY['system','importer','operator',
            'embedding','quote_public_service','quote_confirmation_service']::text[])
          AND EXISTS (SELECT 1 FROM agent_keys k WHERE k.tenant_id=tenant
            AND k.principal_id=p.id AND k.revoked_at IS NULL
            AND (k.expires_at IS NULL OR k.expires_at>now())) ORDER BY p.created_at,p.id LOOP
            PERFORM aeon_migrate_agent_binding(tenant,agent_id);
        END LOOP;
        SELECT id INTO owner_role FROM roles WHERE tenant_id=tenant AND key='owner';
        IF NOT EXISTS (SELECT 1 FROM role_bindings b JOIN principals p
             ON p.tenant_id=b.tenant_id AND p.id=b.principal_id
             WHERE b.tenant_id=tenant AND b.role_id=owner_role
               AND b.scope_type='workspace' AND p.status='active') THEN
            SELECT p.id INTO fallback_id FROM principals p JOIN role_bindings b
              ON b.tenant_id=p.tenant_id AND b.principal_id=p.id
              JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
              WHERE p.tenant_id=tenant AND p.kind='person' AND p.status='active'
                AND p.linked_to IS NULL AND b.scope_type='workspace' AND r.key='admin'
              ORDER BY p.created_at,p.id LIMIT 1;
            IF fallback_id IS NULL THEN
                RAISE EXCEPTION 'tenant % has no active owner or admin person', tenant;
            END IF;
            UPDATE role_bindings SET role_id=owner_role
              WHERE tenant_id=tenant AND principal_id=fallback_id AND scope_type='workspace';
            INSERT INTO events(tenant_id,actor_principal_id,type,after)
              VALUES(tenant,aeon_authz_system_actor(tenant),'authz.owner_fallback',
                jsonb_build_object('principal_id',fallback_id,'role_id',owner_role));
        END IF;
    END LOOP;
    PERFORM set_config('aeon.tenant_id',coalesce(prior_setting,''),true);
END;
$$;

CREATE FUNCTION aeon_protect_last_owner() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_role uuid;
DECLARE remaining integer;
BEGIN
    -- The tenant row serializes concurrent removals and deactivations.
    PERFORM 1 FROM tenants WHERE id=OLD.tenant_id FOR UPDATE;
    SELECT id INTO owner_role FROM roles WHERE tenant_id=OLD.tenant_id AND key='owner';
    IF TG_TABLE_NAME='role_bindings' THEN
        IF OLD.role_id IS DISTINCT FROM owner_role OR OLD.scope_type <> 'workspace' OR
           (TG_OP='UPDATE' AND NEW.role_id=owner_role AND NEW.scope_type='workspace') THEN
            RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
        END IF;
    ELSE
        IF OLD.status <> 'active' OR NEW.status='active' THEN RETURN NEW; END IF;
        IF NOT EXISTS (SELECT 1 FROM role_bindings WHERE tenant_id=OLD.tenant_id
                       AND principal_id=OLD.id AND role_id=owner_role AND scope_type='workspace') THEN
            RETURN NEW;
        END IF;
    END IF;
    SELECT count(*) INTO remaining FROM role_bindings b
      JOIN principals p ON p.tenant_id=b.tenant_id AND p.id=b.principal_id
      WHERE b.tenant_id=OLD.tenant_id AND b.role_id=owner_role
        AND b.scope_type='workspace' AND p.status='active'
        AND (TG_TABLE_NAME <> 'principals' OR p.id <> OLD.id)
        AND (TG_TABLE_NAME <> 'role_bindings' OR b.id <> OLD.id);
    IF remaining=0 THEN RAISE EXCEPTION 'cannot remove the last active owner' USING ERRCODE='23514'; END IF;
    RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END;
$$;
CREATE TRIGGER role_bindings_last_owner BEFORE UPDATE OR DELETE ON role_bindings
    FOR EACH ROW EXECUTE FUNCTION aeon_protect_last_owner();
CREATE TRIGGER principals_last_owner BEFORE UPDATE OF status ON principals
    FOR EACH ROW EXECUTE FUNCTION aeon_protect_last_owner();

CREATE FUNCTION aeon_revoke_deactivated() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status='active' AND NEW.status='deactivated' THEN
        DELETE FROM sessions WHERE tenant_id=NEW.tenant_id AND principal_id=NEW.id;
        UPDATE agent_keys SET revoked_at=coalesce(revoked_at,now())
            WHERE tenant_id=NEW.tenant_id AND principal_id=NEW.id;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER principals_revoke_deactivated AFTER UPDATE OF status ON principals
    FOR EACH ROW EXECUTE FUNCTION aeon_revoke_deactivated();
