-- SPDX-License-Identifier: AGPL-3.0-only
-- Saved views belong to one project's ticket list (NULL keeps the earlier
-- workspace-wide meaning), carry the whole list shape (multi-key sort such as
-- "state,-updated_at" and a grouping) and are deleted softly, so a delete can be
-- undone and links to the view keep their id. Tenant RLS stays the boundary;
-- owner and share rules remain in the handlers.
ALTER TABLE saved_views
    ADD COLUMN project_id uuid,
    ADD COLUMN sort_keys text[] NOT NULL DEFAULT ARRAY[]::text[],
    ADD COLUMN group_by text NOT NULL DEFAULT 'none',
    ADD COLUMN deleted_at timestamptz,
    ADD CONSTRAINT saved_views_project_fk FOREIGN KEY (tenant_id, project_id) REFERENCES nodes(tenant_id, id),
    ADD CONSTRAINT saved_views_sort_keys_check CHECK (array_position(sort_keys, NULL) IS NULL AND cardinality(sort_keys) <= 8),
    ADD CONSTRAINT saved_views_group_by_check CHECK (group_by ~ '^[a-z_]{1,24}$');

CREATE INDEX saved_views_project_idx ON saved_views (tenant_id, project_id, created_at, id) WHERE deleted_at IS NULL;
