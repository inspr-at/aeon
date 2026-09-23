-- SPDX-License-Identifier: AGPL-3.0-only
-- The classic importer checks each record for an earlier import event by
-- classic_ref. Without this index that check scans every event of the tenant,
-- so a full PPM import slows quadratically with its own progress.
CREATE INDEX events_classic_ref_idx
    ON events (tenant_id, type, (after->>'classic_ref'))
    WHERE after ? 'classic_ref';
