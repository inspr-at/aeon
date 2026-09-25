-- SPDX-License-Identifier: AGPL-3.0-only
-- The customer page orders one configured node kind by title. Resolve its kind
-- once, then read the first page in order without scanning every tenant node.
CREATE INDEX nodes_live_kind_title_idx ON nodes(tenant_id, kind_id, title, id)
    WHERE deleted_at IS NULL;
