-- SPDX-License-Identifier: AGPL-3.0-only
-- P5.4: private classic routing around the R2 principal inbox.
CREATE TABLE inbox_message_targets (
 tenant_id uuid NOT NULL REFERENCES tenants(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL, principal_id uuid NOT NULL,
 address text NOT NULL, adapter text NOT NULL CHECK (adapter IN ('codex','agentd_codex','agentd_claude','agentd_pi','agentd_cursor','grok_bot_routine','claude_resume','claude_channel')),
 target_kind text NOT NULL, maximum_level text NOT NULL CHECK (maximum_level IN ('simple','steer')),
 role text NOT NULL CHECK (role IN ('primary','simple_fallback')),
 version integer NOT NULL CHECK (version > 0), enabled boolean NOT NULL DEFAULT true,
 sealed_target bytea NOT NULL, has_secret boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,id),
 UNIQUE (tenant_id,project_id,address,role,version),
 FOREIGN KEY (tenant_id,project_id) REFERENCES nodes(tenant_id,id),
 FOREIGN KEY (tenant_id,principal_id) REFERENCES principals(tenant_id,id)
);
CREATE UNIQUE INDEX inbox_message_target_active ON inbox_message_targets(tenant_id,project_id,address,role) WHERE enabled;

-- Held bodies never enter inbox_messages (including its R2 listen/SSE paths).
CREATE TABLE inbox_compat_messages (
 tenant_id uuid NOT NULL REFERENCES tenants(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL, sender_principal_id uuid NOT NULL, recipient_principal_id uuid NOT NULL,
 recipient_address text NOT NULL, body text NOT NULL CHECK (length(body) BETWEEN 1 AND 65536),
 key_digest text NOT NULL, request_digest text NOT NULL,
 reply_to_id uuid, inbox_message_id uuid, sent_event_id bigint NOT NULL,
 is_action_request boolean NOT NULL, expects_reply boolean NOT NULL,
 delivery_level text NOT NULL CHECK (delivery_level IN ('simple','steer')),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,id), UNIQUE (tenant_id,project_id,sender_principal_id,key_digest),
 FOREIGN KEY (tenant_id,project_id) REFERENCES nodes(tenant_id,id),
 FOREIGN KEY (tenant_id,sender_principal_id) REFERENCES principals(tenant_id,id),
 FOREIGN KEY (tenant_id,recipient_principal_id) REFERENCES principals(tenant_id,id),
 FOREIGN KEY (tenant_id,reply_to_id) REFERENCES inbox_compat_messages(tenant_id,id),
 FOREIGN KEY (tenant_id,inbox_message_id) REFERENCES inbox_messages(tenant_id,id),
 FOREIGN KEY (tenant_id,sent_event_id) REFERENCES events(tenant_id,id),
 CHECK (sender_principal_id <> recipient_principal_id),
 CHECK (is_action_request = (inbox_message_id IS NULL))
);
CREATE INDEX inbox_compat_recipient ON inbox_compat_messages(tenant_id,project_id,recipient_principal_id,sent_event_id);
CREATE TABLE inbox_reply_obligations (
 tenant_id uuid NOT NULL REFERENCES tenants(id), message_id uuid NOT NULL,
 reply_message_id uuid, opened_at timestamptz NOT NULL DEFAULT now(), closed_at timestamptz,
 PRIMARY KEY (tenant_id,message_id),
 FOREIGN KEY (tenant_id,message_id) REFERENCES inbox_compat_messages(tenant_id,id),
 FOREIGN KEY (tenant_id,reply_message_id) REFERENCES inbox_compat_messages(tenant_id,id),
 CHECK ((closed_at IS NULL) = (reply_message_id IS NULL))
);
CREATE TABLE inbox_message_deliveries (
 tenant_id uuid NOT NULL REFERENCES tenants(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 message_id uuid NOT NULL, target_id uuid, fallback_target_id uuid,
 state text NOT NULL CHECK (state IN ('pending','blocked','held','delivered','dead')),
 reason text NOT NULL CHECK (reason IN ('','target_missing','action_request','unsupported','unavailable')),
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,id), UNIQUE (tenant_id,message_id),
 FOREIGN KEY (tenant_id,message_id) REFERENCES inbox_compat_messages(tenant_id,id),
 FOREIGN KEY (tenant_id,target_id) REFERENCES inbox_message_targets(tenant_id,id),
 FOREIGN KEY (tenant_id,fallback_target_id) REFERENCES inbox_message_targets(tenant_id,id)
);
DO $$
DECLARE tbl text;
BEGIN
 FOREACH tbl IN ARRAY ARRAY['inbox_message_targets','inbox_compat_messages','inbox_reply_obligations','inbox_message_deliveries'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',tbl);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',tbl);
  EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''aeon.tenant_id'',true),'''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''aeon.tenant_id'',true),'''')::uuid)',tbl);
 END LOOP;
END $$;
