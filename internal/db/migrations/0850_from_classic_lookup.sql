-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-175: imported numeric project and issue IDs live in fields.classic.
-- The tenant predicate and live-row predicate match the resolver's lookup.
CREATE INDEX nodes_classic_id_idx ON nodes
    (tenant_id, (fields->'classic'->>'id'))
    WHERE deleted_at IS NULL AND fields->'classic'->>'id' IS NOT NULL;

-- The offline classic offer import maps customer and offer IDs separately.
CREATE INDEX paimos_offer_imports_classic_id_idx ON paimos_offer_imports
    (tenant_id, source_kind, source_id);
