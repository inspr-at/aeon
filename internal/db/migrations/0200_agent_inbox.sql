-- SPDX-License-Identifier: AGPL-3.0-only
-- R2 durable principal inbox. Webhooks are wake hints, never message carriers.
CREATE TABLE inbox_messages (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    sender_principal_id uuid NOT NULL,
    recipient_principal_id uuid NOT NULL,
    reply_to_id uuid,
    sent_event_id bigint NOT NULL,
    body text NOT NULL CHECK (length(body) BETWEEN 1 AND 65536),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    acked_at timestamptz,
    acked_by_principal_id uuid,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, sender_principal_id, idempotency_key),
    FOREIGN KEY (tenant_id, sender_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, recipient_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, reply_to_id) REFERENCES inbox_messages(tenant_id, id),
    FOREIGN KEY (tenant_id, sent_event_id) REFERENCES events(tenant_id, id),
    FOREIGN KEY (tenant_id, acked_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK ((acked_at IS NULL) = (acked_by_principal_id IS NULL)),
    CHECK (expires_at IS NULL OR expires_at > created_at),
    CHECK (sender_principal_id <> recipient_principal_id)
);
CREATE UNIQUE INDEX inbox_messages_event_idx ON inbox_messages(tenant_id, sent_event_id);
CREATE INDEX inbox_messages_pending_idx ON inbox_messages
    (tenant_id, recipient_principal_id, created_at, id) WHERE acked_at IS NULL;
CREATE INDEX inbox_messages_reply_idx ON inbox_messages(tenant_id, reply_to_id)
    WHERE reply_to_id IS NOT NULL;
ALTER TABLE inbox_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY inbox_messages_tenant ON inbox_messages
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE inbox_delivery_targets (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('pull', 'webhook')),
    webhook_url text,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, principal_id) REFERENCES principals(tenant_id, id),
    CHECK ((kind = 'webhook') = (webhook_url IS NOT NULL)),
    CHECK (webhook_url IS NULL OR (length(webhook_url) <= 2048 AND webhook_url ~ '^https://'))
);
CREATE UNIQUE INDEX inbox_one_pull_target ON inbox_delivery_targets(tenant_id, principal_id)
    WHERE kind = 'pull';
CREATE INDEX inbox_targets_principal_idx ON inbox_delivery_targets(tenant_id, principal_id, id);
ALTER TABLE inbox_delivery_targets ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_delivery_targets FORCE ROW LEVEL SECURITY;
CREATE POLICY inbox_delivery_targets_tenant ON inbox_delivery_targets
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE inbox_wakes (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    message_id uuid NOT NULL,
    target_id uuid NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    delivered_at timestamptz,
    last_status integer CHECK (last_status BETWEEN 100 AND 599),
    PRIMARY KEY (tenant_id, message_id, target_id),
    FOREIGN KEY (tenant_id, message_id) REFERENCES inbox_messages(tenant_id, id),
    FOREIGN KEY (tenant_id, target_id) REFERENCES inbox_delivery_targets(tenant_id, id)
);
CREATE INDEX inbox_wakes_queue_idx ON inbox_wakes(tenant_id, next_attempt_at)
    WHERE delivered_at IS NULL;
ALTER TABLE inbox_wakes ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_wakes FORCE ROW LEVEL SECURITY;
CREATE POLICY inbox_wakes_tenant ON inbox_wakes
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_inbox_wake() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM inbox_messages m JOIN inbox_delivery_targets t
          ON t.tenant_id = m.tenant_id AND t.principal_id = m.recipient_principal_id
        WHERE m.tenant_id = NEW.tenant_id AND m.id = NEW.message_id
          AND t.id = NEW.target_id AND t.kind = 'webhook' AND t.enabled
    ) THEN
        RAISE EXCEPTION 'wake target does not belong to message recipient';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER inbox_wakes_target_guard BEFORE INSERT ON inbox_wakes
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_inbox_wake();
