-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-184. The Projects page asks which agents are working right now: sessions
-- that are starting, working or stopping, not idle, with a heartbeat inside the
-- freshness window. This partial index holds only those candidates, freshest
-- heartbeat first, so GET /api/harness-sessions/live is a short range scan on
-- (tenant, heartbeat) bounded by the window and its LIMIT, never a sort of every
-- open session. An index grants no access: the read still runs as the caller,
-- under the tenant and project row-level security of harness_sessions (FORCE
-- RLS; the owning role has no BYPASSRLS), so nothing here needs a service path.
CREATE INDEX harness_sessions_live_idx ON harness_sessions (tenant_id, heartbeat_at DESC, id DESC)
    WHERE phase IN ('starting', 'working', 'stopping') AND activity <> 'idle';
