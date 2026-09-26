-- SPDX-License-Identifier: AGPL-3.0-only
-- AEON-165: sender hand-off receipt. Acceptance records queued. handed_off
-- and failed are terminal; handed_off_at is written once, at confirmation.
CREATE TABLE inbox_receipts (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    message_id uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('queued', 'handed_off', 'failed')),
    target_id uuid,
    target_version integer CHECK (target_version IS NULL OR target_version > 0),
    adapter text NOT NULL DEFAULT '' CHECK (length(adapter) <= 64),
    address text NOT NULL DEFAULT '' CHECK (length(address) <= 256),
    effective_level text NOT NULL DEFAULT '' CHECK (effective_level IN ('', 'simple', 'steer')),
    handed_off_at timestamptz,
    failure_reason text NOT NULL DEFAULT '' CHECK (length(failure_reason) <= 128),
    PRIMARY KEY (tenant_id, message_id),
    FOREIGN KEY (tenant_id, message_id) REFERENCES inbox_messages(tenant_id, id),
    FOREIGN KEY (tenant_id, target_id) REFERENCES inbox_message_targets(tenant_id, id),
    CHECK ((state = 'handed_off') = (handed_off_at IS NOT NULL)),
    CHECK ((state = 'failed') = (failure_reason <> ''))
);
ALTER TABLE inbox_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON inbox_receipts
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_guard_inbox_receipt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.state IN ('handed_off', 'failed') AND (
        NEW.state IS DISTINCT FROM OLD.state
        OR NEW.handed_off_at IS DISTINCT FROM OLD.handed_off_at
        OR NEW.failure_reason IS DISTINCT FROM OLD.failure_reason
        OR NEW.target_id IS DISTINCT FROM OLD.target_id
        OR NEW.target_version IS DISTINCT FROM OLD.target_version
        OR NEW.adapter IS DISTINCT FROM OLD.adapter
        OR NEW.address IS DISTINCT FROM OLD.address
        OR NEW.effective_level IS DISTINCT FROM OLD.effective_level
        OR NEW.message_id IS DISTINCT FROM OLD.message_id
        OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
    ) THEN
        RAISE EXCEPTION 'inbox receipt state is monotonic' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER inbox_receipts_monotonic
    BEFORE UPDATE ON inbox_receipts
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_inbox_receipt();

-- Adapter failures already report these reasons. The original check rejected
-- them, so a terminal grok_bot_routine failure never persisted.
DO $$
DECLARE cname text;
BEGIN
    SELECT conname INTO cname FROM pg_constraint
    WHERE conrelid = 'inbox_message_deliveries'::regclass AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%target_missing%';
    IF cname IS NOT NULL THEN
        EXECUTE format('ALTER TABLE inbox_message_deliveries DROP CONSTRAINT %I', cname);
    END IF;
END $$;
ALTER TABLE inbox_message_deliveries ADD CONSTRAINT inbox_message_deliveries_reason_check
    CHECK (reason IN ('', 'target_missing', 'action_request', 'unsupported', 'unavailable',
        'target_invalid', 'payload_invalid', 'transport_error', 'http_error'));

-- Keep in step with the Go permission registry (inbox.receipt).
CREATE OR REPLACE FUNCTION aeon_authz_registry_permission(candidate text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
    SELECT EXISTS (
      SELECT 1 FROM (VALUES
        ('nodes','read write delete move restore configure'),
        ('kinds','read manage'), ('tags','read write manage'),
        ('relations','read write delete'), ('comments','read write delete'),
        ('attachments','read write delete'), ('knowledge','read write delete'),
        ('journey','read act manage'), ('requirements','read write agree'),
        ('releases','read write deploy'), ('intake','read write decide'),
        ('stage_handoffs','read write decide'), ('harness','read write worker control manage'),
        ('work_orders','read write assign'), ('runs','read write control claim'),
        ('run','create read claim telemetry'), ('account','read manage route probe'),
        ('approvals','read request propose decide decide_high revoke'), ('inbox','read send manage receipt'),
        ('stage','prepare deploy verify apply'), ('models','read manage resolve'),
        ('plugins','read manage invoke'), ('imports','read manage'),
        ('views','read write share'), ('events','read undo undo_other'),
        ('search','read'), ('hours','read write approve'),
        ('quotes','read write issue accept delete manage portal_read portal_accept'),
        ('crm','read write manage'), ('cost_units','read write manage'),
        ('project_groups','read write'), ('profile','read write manage portal_read portal_write'),
        ('settings','read manage'), ('members','read manage'),
        ('roles','read manage'), ('keys','read manage'),
        ('audit','read'), ('authz','read'), ('ownership','transfer')
      ) AS registry(resource,actions)
      CROSS JOIN LATERAL unnest(string_to_array(registry.actions,' ')) AS a(action)
      WHERE candidate=registry.resource || '.' || a.action
    );
$$;
