-- SPDX-License-Identifier: AGPL-3.0-only
-- Resolve selectors with the caller's FORCE RLS context. A migration owner may
-- be a superuser, so SECURITY DEFINER could otherwise bypass that policy.
ALTER FUNCTION aeon_resolve_quote_public_tenant(text) SECURITY INVOKER;
