-- SPDX-License-Identifier: AGPL-3.0-only
-- CP1: fenced local delivery and a durable receiver cursor.
ALTER TABLE inbox_compat_messages
 ADD COLUMN sender_address text NOT NULL DEFAULT '',
 ADD COLUMN thread_id text NOT NULL DEFAULT '',
 ADD COLUMN hop integer NOT NULL DEFAULT 1 CHECK (hop BETWEEN 1 AND 10);
UPDATE inbox_compat_messages c SET sender_address='paimos:'||p.name,thread_id=c.id::text
 FROM principals p WHERE p.tenant_id=c.tenant_id AND p.id=c.sender_principal_id;

ALTER TABLE inbox_message_deliveries
 ADD COLUMN lease_token uuid,
 ADD COLUMN lease_until timestamptz,
 ADD COLUMN effective_target_id uuid,
 ADD COLUMN effective_level text CHECK (effective_level IN ('simple','steer')),
 ADD COLUMN fallback_reason text NOT NULL DEFAULT '';

ALTER TABLE inbox_message_deliveries ADD CONSTRAINT inbox_delivery_effective_target_fk
 FOREIGN KEY (tenant_id,effective_target_id) REFERENCES inbox_message_targets(tenant_id,id);

CREATE TABLE inbox_message_cursors (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 project_id uuid NOT NULL,
 principal_id uuid NOT NULL,
 address text NOT NULL,
 adapter text NOT NULL,
 last_event_id bigint NOT NULL DEFAULT 0 CHECK (last_event_id >= 0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,project_id,principal_id,address,adapter),
 FOREIGN KEY (tenant_id,project_id) REFERENCES nodes(tenant_id,id),
 FOREIGN KEY (tenant_id,principal_id) REFERENCES principals(tenant_id,id)
);
ALTER TABLE inbox_message_cursors ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbox_message_cursors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON inbox_message_cursors
 USING (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
 WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
