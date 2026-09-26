-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P2. Guest is a project-only role: an external collaborator sees
-- the projects they are bound to and nothing workspace-wide. P1 mapped the
-- classic "external" label to a workspace Guest binding as a placeholder,
-- because classic project access is not imported. That binding now opens no
-- project, so it is removed (with an audit event by System) and classic
-- external people are no longer given a workspace binding at sign-in or
-- import. An admin grants them project access instead.
CREATE OR REPLACE FUNCTION aeon_bind_legacy_principal(target_tenant uuid,target_principal uuid)
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
      WHEN 1 THEN 'customer' END;
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

-- Workspace bindings may not use the Guest role from now on.
CREATE FUNCTION aeon_guest_project_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.scope_type = 'workspace' AND EXISTS (
        SELECT 1 FROM roles r WHERE r.tenant_id = NEW.tenant_id AND r.id = NEW.role_id
          AND r.builtin AND r.key = 'guest') THEN
        RAISE EXCEPTION 'guest is a project role' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DO $$
DECLARE
    tenant uuid;
    binding record;
    prior_tenant text := current_setting('aeon.tenant_id', true);
    prior_visible text := current_setting('aeon.visible_projects', true);
BEGIN
    FOR tenant IN SELECT id FROM tenants ORDER BY id LOOP
        PERFORM set_config('aeon.tenant_id', tenant::text, true);
        PERFORM set_config('aeon.visible_projects', '*', true);
        FOR binding IN
            SELECT b.id, b.principal_id, r.id AS role_id, r.key, r.name
            FROM role_bindings b JOIN roles r ON r.tenant_id = b.tenant_id AND r.id = b.role_id
            WHERE b.tenant_id = tenant AND b.scope_type = 'workspace' AND r.builtin AND r.key = 'guest'
            ORDER BY b.created_at, b.id
        LOOP
            DELETE FROM role_bindings WHERE tenant_id = tenant AND id = binding.id;
            INSERT INTO events(tenant_id, actor_principal_id, type, before)
            VALUES (tenant, aeon_authz_system_actor(tenant), 'authz.binding_removed',
                jsonb_build_object('principal_id', binding.principal_id, 'scope_type', 'workspace',
                    'project_id', NULL,
                    'role', jsonb_build_object('id', binding.role_id, 'key', binding.key, 'name', binding.name),
                    'reason', 'guest is a project role'));
        END LOOP;
    END LOOP;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_tenant, ''), true);
    PERFORM set_config('aeon.visible_projects', coalesce(prior_visible, ''), true);
END;
$$;

CREATE TRIGGER role_bindings_guest_project_only
BEFORE INSERT OR UPDATE OF role_id, scope_type ON role_bindings
FOR EACH ROW EXECUTE FUNCTION aeon_guest_project_only();
