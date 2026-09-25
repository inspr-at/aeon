-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P1. Built-in role permissions live in the versioned Go registry.
ALTER TABLE principals ADD COLUMN status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'deactivated'));

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
SELECT aeon_seed_builtin_roles(id) FROM tenants;

-- Only people get classic workspace bindings. Idempotent for importer reruns;
-- subsequent classic role text never changes an existing binding.
CREATE FUNCTION aeon_bind_legacy_principal(target_tenant uuid,target_principal uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE chosen_role uuid;
DECLARE new_binding uuid;
BEGIN
    SELECT r.id INTO chosen_role FROM principals p JOIN roles r ON r.tenant_id=p.tenant_id
      AND r.key=CASE
        WHEN 'super_admin'=ANY(p.roles) THEN 'owner'
        WHEN 'admin'=ANY(p.roles) THEN 'admin'
        WHEN 'member'=ANY(p.roles) OR 'reviewer'=ANY(p.roles) THEN 'member'
        WHEN 'external'=ANY(p.roles) THEN 'guest'
        WHEN 'customer'=ANY(p.roles) THEN 'customer'
      END
    WHERE p.tenant_id=target_tenant AND p.id=target_principal AND p.kind='person';
    IF chosen_role IS NULL THEN RETURN; END IF;
    INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type)
      VALUES(target_tenant,target_principal,chosen_role,'workspace')
      ON CONFLICT DO NOTHING RETURNING id INTO new_binding;
    IF new_binding IS NOT NULL THEN
      INSERT INTO events(tenant_id,actor_principal_id,type,after)
      VALUES(target_tenant,target_principal,'authz.binding_migrated',
        jsonb_build_object('principal_id',target_principal,'role_id',chosen_role,'scope_type','workspace'));
    END IF;
END;
$$;
SELECT aeon_bind_legacy_principal(tenant_id,id) FROM principals WHERE kind='person';

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
