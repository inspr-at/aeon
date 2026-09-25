-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR-003 P1 follow-up. The built-in role seed runs from the tenants insert
-- trigger, but EnsureTenant inserts a tenant without any aeon.tenant_id, so
-- under FORCE ROW LEVEL SECURITY (production's NOBYPASSRLS owner role) the
-- seed insert matched neither roles policy and every new tenant failed. The
-- seed now scopes itself to the tenant it seeds and restores the caller's
-- setting afterwards.
CREATE OR REPLACE FUNCTION aeon_seed_builtin_roles(target uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=public,pg_temp AS $$
DECLARE
    prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    PERFORM set_config('aeon.tenant_id', target::text, true);
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
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting,''), true);
END;
$$;
REVOKE EXECUTE ON FUNCTION aeon_seed_builtin_roles(uuid) FROM PUBLIC;
