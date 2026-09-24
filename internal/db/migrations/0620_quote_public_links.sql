-- SPDX-License-Identifier: AGPL-3.0-only
-- Capability expiry is independent of the frozen offer's validity day.
ALTER TABLE quote_public_links
  ADD COLUMN expires_at timestamptz NOT NULL DEFAULT (clock_timestamp() + interval '30 days');
ALTER TABLE quote_public_links
  ADD CONSTRAINT quote_public_links_expiry_order CHECK (expires_at > issued_at);
CREATE INDEX quote_public_links_expiry_idx ON quote_public_links(tenant_id, expires_at)
  WHERE revoked_at IS NULL;

ALTER TABLE quote_public_acceptances
  ADD COLUMN client_mutation_id uuid NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX quote_public_acceptances_replay_idx
  ON quote_public_acceptances(tenant_id,public_link_id,client_mutation_id);

-- The opaque tenant selector must be resolved before its tenant ID is known.
-- Invoke this only through db.InTenant using the sentinel tenant. A guessed
-- selector reveals no quote, person, or token data.
CREATE FUNCTION aeon_resolve_quote_public_tenant(p_selector text)
RETURNS uuid LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT tenant_id FROM quote_public_tenant_selectors WHERE selector = p_selector
$$;

-- Restricted evidence and link metadata are never projected into generic
-- node/event APIs. Reversal is possible only for an active link, with an event.
CREATE FUNCTION aeon_guard_public_link_expiry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.expires_at IS DISTINCT FROM OLD.expires_at THEN
    RAISE EXCEPTION 'public link expiry is immutable';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER quote_public_links_expiry_guard BEFORE UPDATE ON quote_public_links
  FOR EACH ROW EXECUTE FUNCTION aeon_guard_public_link_expiry();
