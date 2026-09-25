-- SPDX-License-Identifier: AGPL-3.0-only
-- Go backfills ciphertext for each tenant through db.InTenant before 0771.
-- The server does not become available between these migrations.
ALTER TABLE quote_public_link_tokens ADD COLUMN ciphertext bytea;
