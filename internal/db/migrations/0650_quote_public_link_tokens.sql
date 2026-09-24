-- SPDX-License-Identifier: AGPL-3.0-only
-- The public-link projection keeps only a verifier. This tenant-scoped vault
-- retains the capability for an admin to copy or print the same issued link
-- again. Public readers never query this table.
CREATE TABLE quote_public_link_tokens (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 link_id uuid NOT NULL,
 token text NOT NULL CHECK (token ~ '^[A-Za-z0-9_-]{40,80}$'),
 PRIMARY KEY (tenant_id, link_id),
 FOREIGN KEY (tenant_id, link_id) REFERENCES quote_public_links(tenant_id, id)
);
ALTER TABLE quote_public_link_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_public_link_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_public_link_tokens_tenant ON quote_public_link_tokens
 USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
