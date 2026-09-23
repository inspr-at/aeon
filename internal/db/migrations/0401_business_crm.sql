-- SPDX-License-Identifier: AGPL-3.0-only
-- CRM identity remains in tenant-configured organisation/contact nodes.
-- Directed graph links keep their meaning across projects and quotes.
ALTER TABLE node_relations DROP CONSTRAINT node_relations_type_check;
ALTER TABLE node_relations ADD CONSTRAINT node_relations_type_check
    CHECK (type IN ('blocks', 'relates', 'implements', 'cites', 'duplicates',
                   'customer_of', 'contact_for'));

-- A contact may represent a signed-in customer. Binding is explicit; matching
-- an email or a display name is never sufficient for quote acceptance.
CREATE TABLE crm_contact_principals (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    contact_node_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    bound_by_principal_id uuid NOT NULL,
    bound_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, contact_node_id, principal_id),
    FOREIGN KEY (tenant_id, contact_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, bound_by_principal_id) REFERENCES principals(tenant_id, id)
);
CREATE INDEX crm_contact_principals_principal_idx ON crm_contact_principals
    (tenant_id, principal_id, contact_node_id);
ALTER TABLE crm_contact_principals ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_contact_principals FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_contact_principals_tenant ON crm_contact_principals
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_crm_contact_principal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT aeon_business_node_kind(NEW.tenant_id, NEW.contact_node_id, 'contact')
       OR NOT EXISTS (SELECT 1 FROM principals p WHERE p.tenant_id = NEW.tenant_id
                     AND p.id = NEW.principal_id AND p.kind = 'person') THEN
        RAISE EXCEPTION 'CRM binding requires a live contact and person';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER crm_contact_principals_guard BEFORE INSERT OR UPDATE OF contact_node_id, principal_id
    ON crm_contact_principals FOR EACH ROW EXECUTE FUNCTION aeon_guard_crm_contact_principal();
