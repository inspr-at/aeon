-- SPDX-License-Identifier: AGPL-3.0-only
-- Historical keys remain tenant-local identities after an issue changes project.
CREATE TABLE node_key_aliases (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    key text NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}-[1-9][0-9]*$' AND length(key) <= 30),
    node_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key),
    FOREIGN KEY (tenant_id, node_id) REFERENCES nodes(tenant_id, id)
);
CREATE INDEX node_key_aliases_node_idx ON node_key_aliases(tenant_id, node_id);
ALTER TABLE node_key_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE node_key_aliases FORCE ROW LEVEL SECURITY;
CREATE POLICY node_key_aliases_tenant ON node_key_aliases
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_node_key_not_aliased() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM node_key_aliases a WHERE a.tenant_id=NEW.tenant_id AND a.key=NEW.key) THEN
        RAISE EXCEPTION 'node key is reserved by alias';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER nodes_key_alias_guard BEFORE INSERT OR UPDATE OF key ON nodes
    FOR EACH ROW EXECUTE FUNCTION aeon_node_key_not_aliased();

CREATE FUNCTION aeon_alias_key_not_current() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM nodes n WHERE n.tenant_id=NEW.tenant_id AND n.key=NEW.key) THEN
        RAISE EXCEPTION 'node key is current';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER node_key_alias_current_guard BEFORE INSERT OR UPDATE OF key ON node_key_aliases
    FOR EACH ROW EXECUTE FUNCTION aeon_alias_key_not_current();
