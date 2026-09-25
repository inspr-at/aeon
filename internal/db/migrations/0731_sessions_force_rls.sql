-- SPDX-License-Identifier: AGPL-3.0-only
-- Session creation is tenant scoped. Authentication knows only the cookie hash,
-- so its sentinel transaction may update or delete exactly that matching row.
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY sessions_tenant ON sessions
  USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE POLICY sessions_lookup_select ON sessions FOR SELECT
  USING (current_setting('aeon.tenant_id', true) = '00000000-0000-0000-0000-000000000000'
    AND id = current_setting('aeon.session_id', true));
CREATE POLICY sessions_lookup_update ON sessions FOR UPDATE
  USING (current_setting('aeon.tenant_id', true) = '00000000-0000-0000-0000-000000000000'
    AND id = current_setting('aeon.session_id', true))
  WITH CHECK (current_setting('aeon.tenant_id', true) = '00000000-0000-0000-0000-000000000000'
    AND id = current_setting('aeon.session_id', true));
CREATE POLICY sessions_lookup_delete ON sessions FOR DELETE
  USING (current_setting('aeon.tenant_id', true) = '00000000-0000-0000-0000-000000000000'
    AND id = current_setting('aeon.session_id', true));
