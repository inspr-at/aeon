-- SPDX-License-Identifier: AGPL-3.0-only
-- P5 consumes these projections; P1 creates no public handler or token.
CREATE TABLE quote_public_tenant_selectors (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
 selector text NOT NULL UNIQUE CHECK (selector ~ '^[A-Za-z0-9_-]{20,80}$')
);
ALTER TABLE quote_public_tenant_selectors ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_public_tenant_selectors FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_public_tenant_selectors_tenant ON quote_public_tenant_selectors USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_public_links (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 id uuid NOT NULL DEFAULT gen_random_uuid(),
 quote_node_id uuid NOT NULL,
 version integer NOT NULL,
 token_sha256 text NOT NULL CHECK (token_sha256 ~ '^[0-9a-f]{64}$'),
 target_content_sha256 text NOT NULL CHECK (target_content_sha256 ~ '^[0-9a-f]{64}$'),
 issued_at timestamptz NOT NULL DEFAULT now(),
 issued_by_principal_id uuid NOT NULL,
 issued_event_id bigint NOT NULL,
 revoked_at timestamptz,
 revoked_by_principal_id uuid,
 revoked_event_id bigint,
 PRIMARY KEY (tenant_id,id),
 UNIQUE (tenant_id,token_sha256),
 FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_issues(tenant_id,quote_node_id,version),
 FOREIGN KEY (tenant_id,issued_by_principal_id) REFERENCES principals(tenant_id,id),
 FOREIGN KEY (tenant_id,issued_event_id) REFERENCES events(tenant_id,id),
 FOREIGN KEY (tenant_id,revoked_by_principal_id) REFERENCES principals(tenant_id,id),
 FOREIGN KEY (tenant_id,revoked_event_id) REFERENCES events(tenant_id,id),
 CHECK ((revoked_at IS NULL AND revoked_by_principal_id IS NULL AND revoked_event_id IS NULL) OR (revoked_at IS NOT NULL AND revoked_by_principal_id IS NOT NULL AND revoked_event_id IS NOT NULL))
);
CREATE UNIQUE INDEX quote_public_links_one_active ON quote_public_links(tenant_id,quote_node_id,version) WHERE revoked_at IS NULL;
ALTER TABLE quote_public_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_public_links FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_public_links_tenant ON quote_public_links USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE FUNCTION aeon_guard_quote_public_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM quote_versions v JOIN business_quotes q ON q.tenant_id=v.tenant_id AND q.quote_node_id=v.quote_node_id
   WHERE v.tenant_id=NEW.tenant_id AND v.quote_node_id=NEW.quote_node_id AND v.version=NEW.version
     AND v.content_sha256=NEW.target_content_sha256 AND q.current_version=v.version AND q.state='issued') THEN
  RAISE EXCEPTION 'public link must bind issued digest';
 END IF;
 IF NEW.revoked_at IS NOT NULL OR NOT EXISTS (SELECT 1 FROM events e WHERE e.tenant_id=NEW.tenant_id AND e.id=NEW.issued_event_id AND e.type='quote.public_link_created' AND e.actor_principal_id=NEW.issued_by_principal_id) THEN
  RAISE EXCEPTION 'public link requires creation event and active state';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER quote_public_links_guard BEFORE INSERT ON quote_public_links FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_public_link();

-- A single decision is shared by both channels. The authenticated R4 insert
-- trigger below keeps its existing person/contact/digest checks intact.
CREATE TABLE quote_decisions (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 quote_node_id uuid NOT NULL,
 version integer NOT NULL,
 channel text NOT NULL CHECK (channel IN ('authenticated','public')),
 event_id bigint NOT NULL,
 decided_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,quote_node_id,version),
 UNIQUE (tenant_id,event_id),
 FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_issues(tenant_id,quote_node_id,version),
 FOREIGN KEY (tenant_id,event_id) REFERENCES events(tenant_id,id)
);
ALTER TABLE quote_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_decisions FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_decisions_tenant ON quote_decisions USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
INSERT INTO quote_decisions(tenant_id,quote_node_id,version,channel,event_id,decided_at)
 SELECT tenant_id,quote_node_id,version,'authenticated',event_id,accepted_at FROM quote_acceptances;
CREATE FUNCTION aeon_record_authenticated_quote_decision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO quote_decisions(tenant_id,quote_node_id,version,channel,event_id)
 VALUES(NEW.tenant_id,NEW.quote_node_id,NEW.version,'authenticated',NEW.event_id);
 RETURN NEW;
END;
$$;
CREATE TRIGGER quote_acceptances_common_decision AFTER INSERT ON quote_acceptances FOR EACH ROW EXECUTE FUNCTION aeon_record_authenticated_quote_decision();

CREATE TABLE quote_public_acceptances (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 quote_node_id uuid NOT NULL,
 version integer NOT NULL,
 public_link_id uuid NOT NULL,
 accepted_content_sha256 text NOT NULL CHECK (accepted_content_sha256 ~ '^[0-9a-f]{64}$'),
 accepted_at timestamptz NOT NULL DEFAULT now(),
 accepted_name text NOT NULL CHECK (length(btrim(accepted_name)) BETWEEN 1 AND 500),
 accepted_company text NOT NULL DEFAULT '',
 accepted_note text NOT NULL DEFAULT '',
 evidence_sha256 text NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
 restricted_audit jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(restricted_audit)='object'),
 event_id bigint NOT NULL,
 PRIMARY KEY (tenant_id,quote_node_id,version),
 UNIQUE (tenant_id,event_id),
 FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_issues(tenant_id,quote_node_id,version),
 FOREIGN KEY (tenant_id,public_link_id) REFERENCES quote_public_links(tenant_id,id),
 FOREIGN KEY (tenant_id,event_id) REFERENCES events(tenant_id,id)
);
ALTER TABLE quote_public_acceptances ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_public_acceptances FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_public_acceptances_tenant ON quote_public_acceptances USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE FUNCTION aeon_guard_quote_public_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM quote_public_links l
  JOIN quote_versions v ON v.tenant_id=l.tenant_id AND v.quote_node_id=l.quote_node_id AND v.version=l.version
  JOIN business_quotes q ON q.tenant_id=v.tenant_id AND q.quote_node_id=v.quote_node_id
  LEFT JOIN quote_version_snapshots s ON s.tenant_id=v.tenant_id AND s.quote_node_id=v.quote_node_id AND s.version=v.version
  WHERE l.tenant_id=NEW.tenant_id AND l.id=NEW.public_link_id AND l.quote_node_id=NEW.quote_node_id AND l.version=NEW.version
   AND l.revoked_at IS NULL AND q.current_version=v.version AND q.state='issued'
   AND l.target_content_sha256=v.content_sha256 AND NEW.accepted_content_sha256=v.content_sha256
   AND (s.valid_until IS NULL OR s.valid_until >= (now() AT TIME ZONE s.validity_time_zone)::date)
   AND EXISTS (SELECT 1 FROM events e WHERE e.tenant_id=NEW.tenant_id AND e.id=NEW.event_id AND e.type='quote.accepted_public')
 ) THEN RAISE EXCEPTION 'public acceptance requires current issued unexpired version and link'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER quote_public_acceptances_guard BEFORE INSERT ON quote_public_acceptances FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_public_acceptance();
CREATE FUNCTION aeon_record_public_quote_decision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO quote_decisions(tenant_id,quote_node_id,version,channel,event_id)
 VALUES(NEW.tenant_id,NEW.quote_node_id,NEW.version,'public',NEW.event_id);
 RETURN NEW;
END;
$$;
CREATE TRIGGER quote_public_acceptances_common_decision AFTER INSERT ON quote_public_acceptances FOR EACH ROW EXECUTE FUNCTION aeon_record_public_quote_decision();
CREATE TRIGGER quote_public_acceptances_immutable BEFORE UPDATE OR DELETE ON quote_public_acceptances FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER quote_decisions_immutable BEFORE UPDATE OR DELETE ON quote_decisions FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
