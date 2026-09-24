-- SPDX-License-Identifier: AGPL-3.0-only
CREATE FUNCTION aeon_guard_quote_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.customer_org_node_id IS DISTINCT FROM OLD.customer_org_node_id
    OR NEW.project_node_id IS DISTINCT FROM OLD.project_node_id
    OR (OLD.offer_no IS NOT NULL AND NEW.offer_no IS DISTINCT FROM OLD.offer_no) THEN
  RAISE EXCEPTION 'quote customer, project and allocated number are immutable';
 END IF;
 IF NEW.state='accepted' AND OLD.state<>'accepted' AND NOT EXISTS (
  SELECT 1 FROM quote_decisions d WHERE d.tenant_id=NEW.tenant_id AND d.quote_node_id=NEW.quote_node_id AND d.version=NEW.current_version
 ) THEN RAISE EXCEPTION 'accepted quote needs one recorded decision'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER business_quotes_identity_guard BEFORE UPDATE ON business_quotes FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_identity();

CREATE FUNCTION aeon_guard_quote_draft_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM business_quotes q WHERE q.tenant_id=NEW.tenant_id AND q.quote_node_id=NEW.quote_node_id AND q.state='draft') THEN
  RAISE EXCEPTION 'only an active draft quote can be edited';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER quote_drafts_update_guard BEFORE UPDATE ON quote_drafts FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_draft_update();

CREATE FUNCTION aeon_guard_customer_number() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT aeon_business_node_kind(NEW.tenant_id,NEW.organisation_node_id,'organisation') THEN RAISE EXCEPTION 'customer number requires live organisation'; END IF;
 IF TG_OP='UPDATE' THEN
  IF NEW.organisation_node_id IS DISTINCT FROM OLD.organisation_node_id
     OR OLD.customer_no !~ '^K[0-9]{2}-[0-9]{3,}$'
     OR NEW.customer_no !~ '^K[0-9]{4}[1-9][0-9]*$'
     OR NEW.provenance <> 'converted'
     OR EXISTS (SELECT 1 FROM business_quotes q WHERE q.tenant_id=NEW.tenant_id AND q.customer_org_node_id=NEW.organisation_node_id AND q.state<>'draft') THEN
   RAISE EXCEPTION 'customer number conversion requires legacy number and draft-only quotes';
  END IF;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER crm_customer_numbers_guard BEFORE INSERT OR UPDATE ON crm_customer_numbers FOR EACH ROW EXECUTE FUNCTION aeon_guard_customer_number();

CREATE FUNCTION aeon_guard_public_link_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.id IS DISTINCT FROM OLD.id
    OR NEW.quote_node_id IS DISTINCT FROM OLD.quote_node_id OR NEW.version IS DISTINCT FROM OLD.version
    OR NEW.token_sha256 IS DISTINCT FROM OLD.token_sha256 OR NEW.target_content_sha256 IS DISTINCT FROM OLD.target_content_sha256
    OR NEW.issued_at IS DISTINCT FROM OLD.issued_at OR NEW.issued_by_principal_id IS DISTINCT FROM OLD.issued_by_principal_id
    OR NEW.issued_event_id IS DISTINCT FROM OLD.issued_event_id
    OR (OLD.revoked_at IS NOT NULL AND (NEW.revoked_at IS DISTINCT FROM OLD.revoked_at OR NEW.revoked_by_principal_id IS DISTINCT FROM OLD.revoked_by_principal_id OR NEW.revoked_event_id IS DISTINCT FROM OLD.revoked_event_id)) THEN
  RAISE EXCEPTION 'public link target and revocation are immutable';
 END IF;
 IF OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM events e WHERE e.tenant_id=NEW.tenant_id AND e.id=NEW.revoked_event_id AND e.type='quote.public_link_revoked' AND e.actor_principal_id=NEW.revoked_by_principal_id
 ) THEN RAISE EXCEPTION 'public link revocation requires event'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER quote_public_links_update_guard BEFORE UPDATE ON quote_public_links FOR EACH ROW EXECUTE FUNCTION aeon_guard_public_link_update();
