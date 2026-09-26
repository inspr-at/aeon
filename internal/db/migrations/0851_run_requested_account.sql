-- SPDX-License-Identifier: AGPL-3.0-only
-- A launch account choice is distinct from a successful allowance reservation.
-- Existing runs retain automatic routing; the existing tenant RLS applies.
ALTER TABLE agent_runs ADD COLUMN requested_account_id uuid;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_requested_account_fk
    FOREIGN KEY (tenant_id, requested_account_id) REFERENCES agent_accounts(tenant_id, id);
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_requested_account_matches
    CHECK (requested_account_id IS NULL OR account_id IS NULL OR requested_account_id = account_id);
