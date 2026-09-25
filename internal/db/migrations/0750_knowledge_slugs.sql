-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-126: knowledge entries are found by project, kind and slug (fields.slug),
-- and an old slug still resolves through the event that renamed it away.
CREATE INDEX nodes_slug_idx ON nodes (tenant_id, kind_id, (fields->>'slug'))
    WHERE deleted_at IS NULL AND fields ? 'slug';
CREATE INDEX events_renamed_slug_idx ON events (tenant_id, (before->'fields'->>'slug'))
    WHERE type IN ('knowledge.updated', 'node.updated') AND before->'fields' ? 'slug';
