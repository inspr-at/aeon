-- SPDX-License-Identifier: AGPL-3.0-only
-- QL1/AEON-109: a draft that was never issued can be deleted from the Quotes
-- list and restored by undoing the event. Deletion is soft: the quote node and
-- its projection are hidden, its commercial number stays allocated (numbers are
-- never reused) and nothing issued, accepted or linked can reach this state.
ALTER TABLE business_quotes ADD COLUMN deleted_at timestamptz;
ALTER TABLE business_quotes ADD COLUMN deleted_by_principal_id uuid;
ALTER TABLE business_quotes ADD CONSTRAINT business_quotes_delete_actor_fk
  FOREIGN KEY (tenant_id, deleted_by_principal_id) REFERENCES principals(tenant_id, id);
ALTER TABLE business_quotes ADD CONSTRAINT business_quotes_delete_actor_pair
  CHECK ((deleted_at IS NULL) = (deleted_by_principal_id IS NULL));

CREATE FUNCTION aeon_guard_quote_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL AND (
    NEW.state <> 'draft' OR NEW.current_version <> 0
    OR EXISTS (SELECT 1 FROM quote_versions v WHERE v.tenant_id = NEW.tenant_id AND v.quote_node_id = NEW.quote_node_id)
 ) THEN
  RAISE EXCEPTION 'only a draft that was never issued can be deleted';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER business_quotes_delete_guard BEFORE UPDATE OF deleted_at ON business_quotes
  FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_delete();
