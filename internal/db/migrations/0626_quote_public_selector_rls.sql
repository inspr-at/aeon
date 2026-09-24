-- SPDX-License-Identifier: AGPL-3.0-only
-- The service role owns the resolver function and FORCE RLS still applies to
-- SECURITY DEFINER. Allow its sentinel-tenant transaction to see only the
-- opaque selector presented for this lookup; all other rows remain hidden.
CREATE POLICY quote_public_tenant_selectors_public_lookup
ON quote_public_tenant_selectors FOR SELECT
USING (
  current_setting('aeon.tenant_id', true) = '00000000-0000-0000-0000-000000000000'
  AND selector = current_setting('aeon.public_quote_selector', true)
);
