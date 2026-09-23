-- SPDX-License-Identifier: AGPL-3.0-only
-- Directed relations and one durable, append-only event log scoped by tenant.
CREATE TABLE node_relations (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    source_node_id uuid NOT NULL,
    target_node_id uuid NOT NULL,
    type text NOT NULL CHECK (type IN ('blocks', 'relates', 'implements', 'cites', 'duplicates')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, source_node_id, target_node_id, type),
    FOREIGN KEY (tenant_id, source_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, target_node_id) REFERENCES nodes(tenant_id, id),
    CHECK (source_node_id <> target_node_id),
    CHECK (type <> 'relates' OR source_node_id < target_node_id)
);
CREATE INDEX node_relations_target_idx ON node_relations(tenant_id, target_node_id, id);
ALTER TABLE node_relations ENABLE ROW LEVEL SECURITY;
ALTER TABLE node_relations FORCE ROW LEVEL SECURITY;
CREATE POLICY node_relations_tenant ON node_relations
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- R0 principals has a global id PK. This additive unique index lets R1 use
-- composite FKs that also prove the actor belongs to the event's tenant.
CREATE UNIQUE INDEX principals_tenant_id_for_r1 ON principals(tenant_id, id);

-- Transactional per-tenant IDs are required for lossless SSE resume. A global
-- identity sequence could commit ID 2 before ID 1 and make ID 1 invisible to
-- clients resuming after 2.
CREATE TABLE event_counters (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
    last_id bigint NOT NULL DEFAULT 0 CHECK (last_id >= 0)
);
ALTER TABLE event_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY event_counters_tenant ON event_counters
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE events (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id bigint NOT NULL,
    actor_principal_id uuid NOT NULL,
    node_id uuid, -- Historical reference: retained after node deletion or undo.
    type text NOT NULL CHECK (type ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    before jsonb,
    after jsonb,
    at timestamptz NOT NULL DEFAULT clock_timestamp(),
    undo_of bigint,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, undo_of), -- One compensating event per original event.
    FOREIGN KEY (tenant_id, actor_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, undo_of) REFERENCES events(tenant_id, id),
    CHECK (before IS NOT NULL OR after IS NOT NULL),
    CHECK (undo_of IS NULL OR undo_of < id)
);
CREATE INDEX events_node_idx ON events(tenant_id, node_id, id) WHERE node_id IS NOT NULL;
CREATE INDEX events_at_idx ON events(tenant_id, at DESC, id DESC);
ALTER TABLE events ENABLE ROW LEVEL SECURITY;
ALTER TABLE events FORCE ROW LEVEL SECURITY;
CREATE POLICY events_read_tenant ON events FOR SELECT
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE POLICY events_insert_tenant ON events FOR INSERT
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_allocate_event_id() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO event_counters(tenant_id, last_id) VALUES (NEW.tenant_id, 1)
    ON CONFLICT (tenant_id) DO UPDATE
        SET last_id = event_counters.last_id + 1
    RETURNING last_id INTO NEW.id;
    RETURN NEW;
END;
$$;
CREATE TRIGGER events_allocate_id
BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION aeon_allocate_event_id();

CREATE FUNCTION aeon_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'events are append-only';
END;
$$;
CREATE TRIGGER events_no_update_delete
BEFORE UPDATE OR DELETE ON events FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE FUNCTION aeon_notify_event() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    -- Notification is a wake-up hint, not the event payload. Consumers replay
    -- committed rows after their last delivered ID to avoid gaps and duplicates.
    PERFORM pg_notify('aeon_events', json_build_object('tenant_id', NEW.tenant_id, 'id', NEW.id)::text);
    RETURN NEW;
END;
$$;
CREATE TRIGGER events_notify
AFTER INSERT ON events FOR EACH ROW EXECUTE FUNCTION aeon_notify_event();
