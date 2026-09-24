-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE quote_confirmation_jobs (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  quote_node_id uuid NOT NULL,
  version integer NOT NULL,
  acceptance_event_id bigint NOT NULL,
  queued_event_id bigint NOT NULL,
  state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','rendering','ready','sending','sent','failed','uncertain')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  lease_until timestamptz,
  last_safe_error text NOT NULL DEFAULT '',
  recipient_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(recipient_snapshot)='object'),
  receipt_sha256 text CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  renderer_version text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,quote_node_id,version),
  FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_decisions(tenant_id,quote_node_id,version),
  FOREIGN KEY (tenant_id,acceptance_event_id) REFERENCES events(tenant_id,id),
  FOREIGN KEY (tenant_id,queued_event_id) REFERENCES events(tenant_id,id),
  CHECK ((receipt_sha256 IS NULL AND renderer_version IS NULL) OR
         (receipt_sha256 IS NOT NULL AND renderer_version IS NOT NULL))
);
CREATE INDEX quote_confirmation_jobs_claim_idx ON quote_confirmation_jobs(tenant_id,next_attempt_at)
  WHERE state IN ('pending','failed');
ALTER TABLE quote_confirmation_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_confirmation_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_confirmation_jobs_tenant ON quote_confirmation_jobs
  USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
  WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_confirmation_receipts (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  quote_node_id uuid NOT NULL,
  version integer NOT NULL,
  acceptance_event_id bigint NOT NULL,
  original_content_sha256 text NOT NULL CHECK (original_content_sha256 ~ '^[0-9a-f]{64}$'),
  file_sha256 text NOT NULL CHECK (file_sha256 ~ '^[0-9a-f]{64}$'),
  renderer_version text NOT NULL,
  stored_event_id bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,quote_node_id,version),
  FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_decisions(tenant_id,quote_node_id,version),
  FOREIGN KEY (tenant_id,acceptance_event_id) REFERENCES events(tenant_id,id),
  FOREIGN KEY (tenant_id,stored_event_id) REFERENCES events(tenant_id,id)
);
ALTER TABLE quote_confirmation_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_confirmation_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_confirmation_receipts_tenant ON quote_confirmation_receipts
  USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
  WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE TRIGGER quote_confirmation_receipts_immutable BEFORE UPDATE OR DELETE ON quote_confirmation_receipts
  FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

-- Both authenticated and public decisions queue the same receipt path. The
-- acceptance event also records this atomic queue transition; no mail is sent.
CREATE FUNCTION aeon_queue_quote_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  -- Historical imports and R4 rate-only versions have no native document
  -- receipt to render. Never synthesize one or queue email from import data.
  IF NOT EXISTS (
    SELECT 1 FROM events e JOIN quote_versions v
      ON v.tenant_id=e.tenant_id AND v.quote_node_id=NEW.quote_node_id AND v.version=NEW.version
    JOIN quote_version_snapshots s
      ON s.tenant_id=v.tenant_id AND s.quote_node_id=v.quote_node_id AND s.version=v.version
    WHERE e.tenant_id=NEW.tenant_id AND e.id=NEW.event_id
      AND e.type IN ('quote.accepted','quote.accepted_public')
      AND v.digest_mode='document-v1'
  ) THEN RETURN NEW; END IF;
  INSERT INTO quote_confirmation_jobs(tenant_id,quote_node_id,version,acceptance_event_id,queued_event_id,recipient_snapshot)
   SELECT NEW.tenant_id,NEW.quote_node_id,NEW.version,NEW.event_id,NEW.event_id,s.recipient
   FROM quote_version_snapshots s
   WHERE s.tenant_id=NEW.tenant_id AND s.quote_node_id=NEW.quote_node_id AND s.version=NEW.version;
  RETURN NEW;
END;
$$;
CREATE TRIGGER quote_decisions_confirmation AFTER INSERT ON quote_decisions
  FOR EACH ROW EXECUTE FUNCTION aeon_queue_quote_confirmation();
