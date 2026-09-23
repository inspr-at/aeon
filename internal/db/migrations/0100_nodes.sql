-- SPDX-License-Identifier: AGPL-3.0-only
-- R1 nodes and tenant-configured kinds. Apply after R0 tenants/principals.
CREATE TABLE node_kinds (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    slug text NOT NULL CHECK (slug ~ '^[a-z][a-z0-9_]*$'),
    label text NOT NULL CHECK (length(btrim(label)) > 0),
    short_prefix text NOT NULL CHECK (short_prefix ~ '^[A-Z][A-Z0-9]{1,9}$'),
    icon text NOT NULL CHECK (length(btrim(icon)) > 0),
    allowed_child_kinds text[], -- NULL permits any kind; empty permits none.
    field_schema jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(field_schema) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, slug),
    CHECK (allowed_child_kinds IS NULL OR array_position(allowed_child_kinds, NULL) IS NULL)
);
ALTER TABLE node_kinds ENABLE ROW LEVEL SECURITY;
ALTER TABLE node_kinds FORCE ROW LEVEL SECURITY;
CREATE POLICY node_kinds_tenant ON node_kinds
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- A tenant-insert trigger seeds new tenants; existing tenants are backfilled
-- below. Slugs are stable identifiers.
CREATE FUNCTION aeon_seed_node_kinds(p_tenant_id uuid) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
    IF p_tenant_id IS DISTINCT FROM NULLIF(current_setting('aeon.tenant_id', true), '')::uuid THEN
        RAISE EXCEPTION 'tenant setting does not match starter-kind tenant';
    END IF;
    INSERT INTO node_kinds (tenant_id, slug, label, short_prefix, icon, field_schema)
    VALUES
        (p_tenant_id, 'project',   'Project',   'PRJ', 'project',   '{}'::jsonb),
        (p_tenant_id, 'epic',      'Epic',      'EPC', 'epic',      '{}'::jsonb),
        (p_tenant_id, 'ticket',    'Ticket',    'TKT', 'ticket',    '{}'::jsonb),
        (p_tenant_id, 'task',      'Task',      'TSK', 'task',      '{}'::jsonb),
        (p_tenant_id, 'release',   'Release',   'REL', 'release',   '{}'::jsonb),
        (p_tenant_id, 'memory',    'Memory',    'MEM', 'memory',    '{}'::jsonb),
        (p_tenant_id, 'runbook',   'Runbook',   'RUN', 'runbook',   '{}'::jsonb),
        (p_tenant_id, 'guideline', 'Guideline', 'GUI', 'guideline', '{}'::jsonb)
    ON CONFLICT (tenant_id, slug) DO NOTHING;
END;
$$;
CREATE FUNCTION aeon_seed_new_tenant_kinds() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    PERFORM set_config('aeon.tenant_id', NEW.id::text, true);
    PERFORM aeon_seed_node_kinds(NEW.id);
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
    RETURN NEW;
EXCEPTION WHEN OTHERS THEN
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
    RAISE;
END;
$$;
CREATE TRIGGER tenants_seed_node_kinds
AFTER INSERT ON tenants FOR EACH ROW EXECUTE FUNCTION aeon_seed_new_tenant_kinds();

DO $$
DECLARE
    t record;
    prior_setting text := current_setting('aeon.tenant_id', true);
BEGIN
    FOR t IN SELECT id FROM tenants LOOP
        PERFORM set_config('aeon.tenant_id', t.id::text, true);
        PERFORM aeon_seed_node_kinds(t.id);
    END LOOP;
    PERFORM set_config('aeon.tenant_id', coalesce(prior_setting, ''), true);
END;
$$;

-- A single counter per (tenant, prefix) allocates keys transactionally. Import
-- inserts preserve their key and advance the same high-water mark.
CREATE TABLE node_key_counters (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    prefix text NOT NULL CHECK (prefix ~ '^[A-Z][A-Z0-9]{1,9}$'),
    last_number bigint NOT NULL DEFAULT 0 CHECK (last_number >= 0),
    PRIMARY KEY (tenant_id, prefix)
);
ALTER TABLE node_key_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE node_key_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY node_key_counters_tenant ON node_key_counters
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_next_node_key(p_tenant_id uuid, p_prefix text) RETURNS text
LANGUAGE plpgsql AS $$
DECLARE
    next_number bigint;
BEGIN
    IF p_tenant_id IS DISTINCT FROM NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
       OR p_prefix !~ '^[A-Z][A-Z0-9]{1,9}$' THEN
        RAISE EXCEPTION 'invalid tenant or key prefix';
    END IF;
    INSERT INTO node_key_counters(tenant_id, prefix, last_number)
    VALUES (p_tenant_id, p_prefix, 1)
    ON CONFLICT (tenant_id, prefix) DO UPDATE
        SET last_number = node_key_counters.last_number + 1
    RETURNING last_number INTO next_number;
    RETURN p_prefix || '-' || next_number;
END;
$$;

CREATE TABLE nodes (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    key text NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}-[1-9][0-9]*$' AND length(key) <= 30),
    kind_id uuid NOT NULL,
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    body text NOT NULL DEFAULT '',
    fields jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(fields) = 'object'),
    state text NOT NULL DEFAULT 'open' CHECK (length(btrim(state)) > 0),
    parent_id uuid,
    position numeric(30, 15) NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, key),
    FOREIGN KEY (tenant_id, kind_id) REFERENCES node_kinds(tenant_id, id),
    FOREIGN KEY (tenant_id, parent_id) REFERENCES nodes(tenant_id, id)
);
CREATE INDEX nodes_siblings_idx ON nodes(tenant_id, parent_id, position, id) WHERE deleted_at IS NULL;
CREATE INDEX nodes_kind_state_idx ON nodes(tenant_id, kind_id, state, id) WHERE deleted_at IS NULL;
CREATE INDEX nodes_updated_idx ON nodes(tenant_id, updated_at DESC, id) WHERE deleted_at IS NULL;
ALTER TABLE nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE nodes FORCE ROW LEVEL SECURITY;
CREATE POLICY nodes_tenant ON nodes
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_validate_node_tree() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    parent_kind_id uuid;
    parent_deleted_at timestamptz;
    allowed text[];
    child_slug text;
    check_link boolean;
BEGIN
    -- Serialize changes to one tenant's tree so concurrent moves cannot form a cycle.
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id::text, 0));
    IF TG_OP = 'INSERT' THEN
        check_link := true;
    ELSE
        check_link := NEW.parent_id IS DISTINCT FROM OLD.parent_id
                   OR NEW.kind_id IS DISTINCT FROM OLD.kind_id;
    END IF;
    IF check_link AND NEW.parent_id IS NOT NULL THEN
        IF NEW.parent_id = NEW.id THEN
            RAISE EXCEPTION 'node cannot parent itself';
        END IF;
        SELECT p.kind_id, p.deleted_at INTO parent_kind_id, parent_deleted_at
        FROM nodes p WHERE p.tenant_id = NEW.tenant_id AND p.id = NEW.parent_id;
        IF NOT FOUND OR parent_deleted_at IS NOT NULL THEN
            RAISE EXCEPTION 'parent node does not exist or is deleted';
        END IF;
        SELECT k.allowed_child_kinds INTO allowed FROM node_kinds k
        WHERE k.tenant_id = NEW.tenant_id AND k.id = parent_kind_id;
        SELECT k.slug INTO child_slug FROM node_kinds k
        WHERE k.tenant_id = NEW.tenant_id AND k.id = NEW.kind_id;
        IF allowed IS NOT NULL AND NOT child_slug = ANY(allowed) THEN
            RAISE EXCEPTION 'child kind is not allowed under parent kind';
        END IF;
        IF EXISTS (
            WITH RECURSIVE ancestors(id, parent_id) AS (
                SELECT p.id, p.parent_id FROM nodes p
                WHERE p.tenant_id = NEW.tenant_id AND p.id = NEW.parent_id
                UNION ALL
                SELECT p.id, p.parent_id FROM nodes p
                JOIN ancestors a ON p.id = a.parent_id
                WHERE p.tenant_id = NEW.tenant_id
            )
            SELECT 1 FROM ancestors WHERE id = NEW.id
        ) THEN
            RAISE EXCEPTION 'node move would create a cycle';
        END IF;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.deleted_at IS NOT NULL
       AND NEW.deleted_at IS NULL AND NOT check_link
       AND NEW.parent_id IS NOT NULL THEN
        PERFORM 1 FROM nodes p WHERE p.tenant_id = NEW.tenant_id
            AND p.id = NEW.parent_id AND p.deleted_at IS NULL;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'cannot restore node under deleted parent';
        END IF;
    END IF;
    IF NEW.deleted_at IS NOT NULL AND (TG_OP = 'INSERT' OR OLD.deleted_at IS NULL) THEN
        IF EXISTS (SELECT 1 FROM nodes c WHERE c.tenant_id = NEW.tenant_id
                   AND c.parent_id = NEW.id AND c.deleted_at IS NULL) THEN
            RAISE EXCEPTION 'node has live children';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER nodes_tree_guard
BEFORE INSERT OR UPDATE OF parent_id, kind_id, deleted_at ON nodes
FOR EACH ROW EXECUTE FUNCTION aeon_validate_node_tree();

CREATE FUNCTION aeon_bump_node_key_counter() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO node_key_counters(tenant_id, prefix, last_number)
    VALUES (NEW.tenant_id, split_part(NEW.key, '-', 1), split_part(NEW.key, '-', 2)::bigint)
    ON CONFLICT (tenant_id, prefix) DO UPDATE
        SET last_number = greatest(node_key_counters.last_number, EXCLUDED.last_number);
    RETURN NEW;
END;
$$;
CREATE TRIGGER nodes_key_counter
AFTER INSERT ON nodes FOR EACH ROW EXECUTE FUNCTION aeon_bump_node_key_counter();
