-- SPDX-License-Identifier: AGPL-3.0-only
-- Backfill runs first; no plaintext capability remains in this table.
ALTER TABLE quote_public_link_tokens ADD CONSTRAINT quote_public_link_tokens_ciphertext_required CHECK (ciphertext IS NOT NULL);
ALTER TABLE quote_public_link_tokens DROP COLUMN token;
