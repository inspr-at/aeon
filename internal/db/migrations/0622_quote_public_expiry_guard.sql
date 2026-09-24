-- SPDX-License-Identifier: AGPL-3.0-only
-- The handler and database enforce the same capability expiry fence. Old
-- offers remain readable through an expired unrevoked link.
CREATE OR REPLACE FUNCTION aeon_guard_quote_public_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM quote_public_links l
  JOIN quote_versions v ON v.tenant_id=l.tenant_id AND v.quote_node_id=l.quote_node_id AND v.version=l.version
  JOIN business_quotes q ON q.tenant_id=v.tenant_id AND q.quote_node_id=v.quote_node_id
  LEFT JOIN quote_version_snapshots s ON s.tenant_id=v.tenant_id AND s.quote_node_id=v.quote_node_id AND s.version=v.version
  WHERE l.tenant_id=NEW.tenant_id AND l.id=NEW.public_link_id AND l.quote_node_id=NEW.quote_node_id AND l.version=NEW.version
   AND l.revoked_at IS NULL AND l.expires_at > now() AND q.current_version=v.version AND q.state='issued'
   AND l.target_content_sha256=v.content_sha256 AND NEW.accepted_content_sha256=v.content_sha256
   AND (s.valid_until IS NULL OR s.valid_until >= (now() AT TIME ZONE s.validity_time_zone)::date)
   AND EXISTS (SELECT 1 FROM events e WHERE e.tenant_id=NEW.tenant_id AND e.id=NEW.event_id AND e.type='quote.accepted_public')
 ) THEN RAISE EXCEPTION 'public acceptance requires current issued unexpired version and link'; END IF;
 RETURN NEW;
END;
$$;
