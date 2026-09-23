-- SPDX-License-Identifier: AGPL-3.0-only
-- B1 list filters and recursive project/within walks.
CREATE INDEX nodes_list_priority_idx ON nodes (tenant_id, (fields->>'priority'), id) WHERE deleted_at IS NULL;
CREATE INDEX nodes_list_assignee_idx ON nodes (tenant_id, (fields->>'assignee'), id) WHERE deleted_at IS NULL;
CREATE INDEX nodes_list_state_idx ON nodes (tenant_id, state, id) WHERE deleted_at IS NULL;
CREATE INDEX nodes_list_parent_updated_idx ON nodes (tenant_id, parent_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX nodes_list_key_natural_idx ON nodes (tenant_id, (regexp_replace(key,'-[0-9]+$','')), ((substring(key from '-([0-9]+)$'))::numeric), id) WHERE deleted_at IS NULL;
