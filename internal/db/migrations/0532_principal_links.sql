-- SPDX-License-Identifier: AGPL-3.0-only
ALTER TABLE principals ADD COLUMN linked_to uuid;
ALTER TABLE principals ADD COLUMN email text;
ALTER TABLE principals ADD CONSTRAINT principal_link_tenant
    FOREIGN KEY (tenant_id, linked_to) REFERENCES principals(tenant_id, id);
ALTER TABLE principals ADD CONSTRAINT principal_link_person
    CHECK (linked_to IS NULL OR (kind = 'person' AND linked_to <> id));
CREATE INDEX principals_linked_to ON principals(tenant_id, linked_to) WHERE linked_to IS NOT NULL;

-- Serialize link topology changes, including callers other than the CLI.
-- FOR UPDATE on endpoints also fences concurrent kind changes/deletion.
CREATE FUNCTION aeon_check_principal_link() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_kind text; target_link uuid;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id::text, 532));
    IF NEW.linked_to IS NOT NULL THEN
        SELECT kind, linked_to INTO target_kind, target_link FROM principals
        WHERE tenant_id=NEW.tenant_id AND id=NEW.linked_to FOR UPDATE;
        IF NOT FOUND OR target_kind <> 'person' OR target_link IS NOT NULL THEN
            RAISE EXCEPTION 'link target must be an unlinked person in the same tenant' USING ERRCODE='23514';
        END IF;
    END IF;
    IF (NEW.linked_to IS NOT NULL OR NEW.kind <> 'person') AND EXISTS (
        SELECT 1 FROM principals WHERE tenant_id=NEW.tenant_id AND linked_to=NEW.id
    ) THEN
        RAISE EXCEPTION 'a link target must remain an unlinked person' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER principal_link_check BEFORE INSERT OR UPDATE OF linked_to, kind, tenant_id ON principals
    FOR EACH ROW EXECUTE FUNCTION aeon_check_principal_link();
