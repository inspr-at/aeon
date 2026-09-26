-- SPDX-License-Identifier: AGPL-3.0-only
-- AC4 / AEON-185: nullable metadata needs no data backfill or RLS bypass.
-- Existing generations stay unlabeled, including under a NOBYPASSRLS owner.
ALTER TABLE harness_sessions ADD COLUMN display_label text
    CHECK (display_label IS NULL OR (char_length(display_label) BETWEEN 1 AND 128
        AND display_label = btrim(display_label) AND display_label !~ '[[:cntrl:]]'));
