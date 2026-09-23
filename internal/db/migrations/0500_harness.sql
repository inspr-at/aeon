-- SPDX-License-Identifier: AGPL-3.0-only
-- Public harness generations are distinct from attribution sessions and agent runs.
CREATE TABLE harness_sessions (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL,
    agent_principal_id uuid NOT NULL,
    run_id uuid,
    ticket_node_id uuid,
    work_order_id uuid,
    parent_id uuid,
    harness text NOT NULL CHECK (harness IN ('codex','claude','pi','cursor','grok')),
    host text NOT NULL CHECK (length(host) BETWEEN 1 AND 128),
    management text NOT NULL CHECK (management IN ('managed','unmanaged')),
    role text NOT NULL CHECK (role IN ('coordinator','worker')),
    work_shape text NOT NULL DEFAULT 'unknown' CHECK (work_shape IN ('unknown','ship','scout')),
    capabilities text[] NOT NULL DEFAULT '{}',
    ref_digest bytea NOT NULL,
    lease_digest bytea NOT NULL,
    phase text NOT NULL DEFAULT 'starting' CHECK (phase IN ('starting','working','yielded','stopping','stopped')),
    activity text NOT NULL DEFAULT 'unknown' CHECK (activity IN ('unknown','busy','idle')),
    activity_sequence bigint NOT NULL DEFAULT 0 CHECK (activity_sequence >= 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    heartbeat_at timestamptz,
    stopped_at timestamptz,
    stop_reason text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id,id),
    UNIQUE (tenant_id,project_id,id),
    FOREIGN KEY (tenant_id,project_id) REFERENCES nodes(tenant_id,id),
    FOREIGN KEY (tenant_id,agent_principal_id) REFERENCES principals(tenant_id,id),
    FOREIGN KEY (tenant_id,run_id) REFERENCES agent_runs(tenant_id,id),
    FOREIGN KEY (tenant_id,ticket_node_id) REFERENCES nodes(tenant_id,id),
    FOREIGN KEY (tenant_id,work_order_id) REFERENCES work_orders(tenant_id,node_id),
    FOREIGN KEY (tenant_id,project_id,parent_id) REFERENCES harness_sessions(tenant_id,project_id,id),
    CHECK (parent_id IS NULL OR parent_id <> id),
    CHECK ((ticket_node_id IS NULL) = (work_shape = 'unknown')),
    CHECK (management = 'managed' OR NOT (capabilities && ARRAY['interrupt','stop']::text[])),
    CHECK ((phase = 'stopped') = (stopped_at IS NOT NULL))
);
CREATE UNIQUE INDEX harness_one_active_ref ON harness_sessions(tenant_id,project_id,ref_digest) WHERE stopped_at IS NULL;
CREATE INDEX harness_project_active ON harness_sessions(tenant_id,project_id,created_at) WHERE stopped_at IS NULL;
CREATE INDEX harness_parent ON harness_sessions(tenant_id,parent_id);
ALTER TABLE harness_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE harness_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY harness_sessions_tenant ON harness_sessions
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE harness_controls (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('interrupt','stop')),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','claimed','completed')),
    sequence bigint NOT NULL CHECK (sequence > 0),
    requested_by_principal_id uuid NOT NULL,
    outcome text CHECK (outcome IN ('applied','rejected')),
    reason text CHECK (reason IS NULL OR length(reason) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    claimed_at timestamptz,
    completed_at timestamptz,
    PRIMARY KEY (tenant_id,id),
    FOREIGN KEY (tenant_id,session_id) REFERENCES harness_sessions(tenant_id,id),
    FOREIGN KEY (tenant_id,requested_by_principal_id) REFERENCES principals(tenant_id,id),
    UNIQUE (tenant_id,session_id,sequence),
    CHECK ((state = 'pending') = (claimed_at IS NULL)),
    CHECK ((state = 'completed') = (completed_at IS NOT NULL)),
    CHECK ((state = 'completed') = (outcome IS NOT NULL))
);
CREATE INDEX harness_controls_pending ON harness_controls(tenant_id,session_id,sequence) WHERE state <> 'completed';
ALTER TABLE harness_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE harness_controls FORCE ROW LEVEL SECURITY;
CREATE POLICY harness_controls_tenant ON harness_controls
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE harness_deliveries (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL,
    message_id uuid NOT NULL,
    cursor bigint NOT NULL CHECK (cursor > 0),
    leased_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    completed_at timestamptz,
    released_at timestamptz,
    PRIMARY KEY (tenant_id,id),
    UNIQUE (tenant_id,session_id,message_id),
    FOREIGN KEY (tenant_id,session_id) REFERENCES harness_sessions(tenant_id,id),
    FOREIGN KEY (tenant_id,message_id) REFERENCES inbox_messages(tenant_id,id),
    CHECK (completed_at IS NULL OR released_at IS NULL)
);
CREATE INDEX harness_deliveries_open ON harness_deliveries(tenant_id,session_id,cursor) WHERE completed_at IS NULL AND released_at IS NULL;
ALTER TABLE harness_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE harness_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY harness_deliveries_tenant ON harness_deliveries
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
