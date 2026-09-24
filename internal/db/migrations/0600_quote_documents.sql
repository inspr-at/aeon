-- SPDX-License-Identifier: AGPL-3.0-only
-- P1 extends R4 without rewriting its immutable version or line bytes.
ALTER TABLE business_quotes ALTER COLUMN project_node_id DROP NOT NULL;
ALTER TABLE business_quotes ADD COLUMN offer_no text;
ALTER TABLE business_quotes ADD COLUMN archived_at timestamptz;
ALTER TABLE business_quotes ADD COLUMN archived_by_principal_id uuid;
ALTER TABLE business_quotes ADD COLUMN project_ref text NOT NULL DEFAULT '';
ALTER TABLE business_quotes ADD CONSTRAINT business_quotes_offer_no_unique UNIQUE (tenant_id, offer_no);
ALTER TABLE business_quotes ADD CONSTRAINT business_quotes_archive_actor_fk FOREIGN KEY (tenant_id, archived_by_principal_id) REFERENCES principals(tenant_id,id);
CREATE OR REPLACE FUNCTION aeon_guard_business_quote() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT aeon_business_node_kind(NEW.tenant_id, NEW.quote_node_id, 'quote')
       OR (NEW.project_node_id IS NOT NULL AND NOT aeon_business_node_kind(NEW.tenant_id, NEW.project_node_id, 'project'))
       OR NOT aeon_business_node_kind(NEW.tenant_id, NEW.customer_org_node_id, 'organisation') THEN
        RAISE EXCEPTION 'quote requires live quote, optional project and organisation nodes';
    END IF;
    RETURN NEW;
END;
$$;

-- A legacy R4 version has a bound contact and digest v1. P1 snapshots can
-- instead freeze a free-text recipient. The companion row is append-only.
ALTER TABLE quote_versions ALTER COLUMN recipient_contact_node_id DROP NOT NULL;
ALTER TABLE quote_versions ADD COLUMN digest_mode text NOT NULL DEFAULT 'r4-v1' CHECK (digest_mode IN ('r4-v1','document-v1'));
ALTER TABLE quote_versions ADD COLUMN pricing_mode text NOT NULL DEFAULT 'rate-4' CHECK (pricing_mode IN ('rate-4','cent-half-up-v1'));
CREATE OR REPLACE FUNCTION aeon_guard_quote_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.recipient_contact_node_id IS NOT NULL AND NOT aeon_business_node_kind(NEW.tenant_id, NEW.recipient_contact_node_id, 'contact') THEN
        RAISE EXCEPTION 'quote recipient must be a live contact';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TABLE quote_settings (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
    revision bigint NOT NULL CHECK (revision > 0),
    numbering_time_zone text NOT NULL CHECK (length(numbering_time_zone) BETWEEN 1 AND 100),
    default_currency text NOT NULL CHECK (default_currency ~ '^[A-Z]{3}$'),
    sender jsonb NOT NULL CHECK (jsonb_typeof(sender)='object'),
    defaults jsonb NOT NULL CHECK (jsonb_typeof(defaults)='object'),
    layout jsonb NOT NULL CHECK (jsonb_typeof(layout)='object'),
    smtp_confirmation_enabled boolean NOT NULL DEFAULT false,
    smtp_configured boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by_principal_id uuid NOT NULL,
    FOREIGN KEY (tenant_id,updated_by_principal_id) REFERENCES principals(tenant_id,id)
);
ALTER TABLE quote_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_settings_tenant ON quote_settings USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_number_sequences (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    kind text NOT NULL CHECK (kind IN ('customer','offer')),
    period text NOT NULL CHECK (period ~ '^[0-9]{4}([0-9]{2})?$'),
    value bigint NOT NULL CHECK (value > 0),
    PRIMARY KEY (tenant_id,kind,period)
);
ALTER TABLE quote_number_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_number_sequences FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_number_sequences_tenant ON quote_number_sequences USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE crm_customer_numbers (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    organisation_node_id uuid NOT NULL,
    customer_no text NOT NULL,
    provenance text NOT NULL DEFAULT 'native' CHECK (provenance IN ('native','imported','converted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,organisation_node_id),
    UNIQUE (tenant_id,customer_no),
    FOREIGN KEY (tenant_id,organisation_node_id) REFERENCES nodes(tenant_id,id)
);
ALTER TABLE crm_customer_numbers ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_customer_numbers FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_customer_numbers_tenant ON crm_customer_numbers USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_drafts (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    document jsonb NOT NULL CHECK (jsonb_typeof(document)='object'),
    schema_version integer NOT NULL CHECK (schema_version > 0),
    minimum_writer_version integer NOT NULL CHECK (minimum_writer_version > 0),
    draft_revision bigint NOT NULL DEFAULT 1 CHECK (draft_revision > 0),
    base_version integer NOT NULL DEFAULT 0 CHECK (base_version >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by_principal_id uuid NOT NULL,
    PRIMARY KEY (tenant_id,quote_node_id),
    FOREIGN KEY (tenant_id,quote_node_id) REFERENCES business_quotes(tenant_id,quote_node_id),
    FOREIGN KEY (tenant_id,updated_by_principal_id) REFERENCES principals(tenant_id,id)
);
ALTER TABLE quote_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_drafts_tenant ON quote_drafts USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_draft_mutations (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    client_session_id uuid NOT NULL,
    mutation_id uuid NOT NULL,
    input_sha256 text NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
    result_revision bigint NOT NULL CHECK (result_revision > 0),
    result_quote_revision bigint NOT NULL CHECK (result_quote_revision > 0),
    event_id bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,quote_node_id,client_session_id,mutation_id),
    FOREIGN KEY (tenant_id,quote_node_id) REFERENCES quote_drafts(tenant_id,quote_node_id),
    FOREIGN KEY (tenant_id,event_id) REFERENCES events(tenant_id,id)
);
ALTER TABLE quote_draft_mutations ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_draft_mutations FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_draft_mutations_tenant ON quote_draft_mutations USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE quote_version_snapshots (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    version integer NOT NULL,
    document jsonb NOT NULL CHECK (jsonb_typeof(document)='object'),
    sender jsonb NOT NULL CHECK (jsonb_typeof(sender)='object'),
    recipient jsonb NOT NULL CHECK (jsonb_typeof(recipient)='object'),
    legal jsonb NOT NULL CHECK (jsonb_typeof(legal)='object'),
    layout jsonb NOT NULL CHECK (jsonb_typeof(layout)='object'),
    offer_no text NOT NULL,
    customer_no text NOT NULL,
    project_ref text NOT NULL,
    offer_date date NOT NULL,
    valid_until date NOT NULL,
    validity_time_zone text NOT NULL,
    document_schema_version integer NOT NULL,
    renderer_version text NOT NULL,
    PRIMARY KEY (tenant_id,quote_node_id,version),
    FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_versions(tenant_id,quote_node_id,version)
);
ALTER TABLE quote_version_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_version_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_version_snapshots_tenant ON quote_version_snapshots USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE TRIGGER quote_version_snapshots_immutable BEFORE UPDATE OR DELETE ON quote_version_snapshots FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE quote_document_lines (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    version integer NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    line_id uuid NOT NULL,
    pricing_source text NOT NULL CHECK (pricing_source IN ('manual','cost_unit')),
    short_text text NOT NULL CHECK (length(btrim(short_text)) > 0),
    long_text text NOT NULL DEFAULT '',
    unit_label text NOT NULL CHECK (length(unit_label) BETWEEN 1 AND 80),
    cost_unit_node_id uuid,
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    quantity numeric(18,4) NOT NULL CHECK (quantity > 0),
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    total_cents bigint NOT NULL CHECK (total_cents >= 0),
    rate_amount numeric(18,4),
    net_amount numeric(18,4) NOT NULL CHECK (net_amount = total_cents::numeric/100),
    tax_rate numeric(8,5) NOT NULL DEFAULT 0 CHECK (tax_rate >= 0 AND tax_rate <= 1),
    PRIMARY KEY (tenant_id,quote_node_id,version,position),
    UNIQUE (tenant_id,quote_node_id,version,line_id),
    FOREIGN KEY (tenant_id,quote_node_id,version) REFERENCES quote_versions(tenant_id,quote_node_id,version),
    FOREIGN KEY (tenant_id,cost_unit_node_id) REFERENCES nodes(tenant_id,id),
    CHECK ((pricing_source='manual' AND cost_unit_node_id IS NULL AND rate_amount IS NULL) OR (pricing_source='cost_unit' AND cost_unit_node_id IS NOT NULL AND rate_amount IS NOT NULL))
);
ALTER TABLE quote_document_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_document_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_document_lines_tenant ON quote_document_lines USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE TRIGGER quote_document_lines_immutable BEFORE UPDATE OR DELETE ON quote_document_lines FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE FUNCTION aeon_guard_quote_document_line() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.cost_unit_node_id IS NOT NULL AND NOT aeon_business_node_kind(NEW.tenant_id,NEW.cost_unit_node_id,'cost_unit'))
       OR EXISTS (SELECT 1 FROM quote_issues WHERE tenant_id=NEW.tenant_id AND quote_node_id=NEW.quote_node_id AND version=NEW.version) THEN
        RAISE EXCEPTION 'document line requires live cost unit when rate-backed and an unissued version';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER quote_document_lines_guard BEFORE INSERT ON quote_document_lines FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_document_line();

CREATE OR REPLACE FUNCTION aeon_guard_quote_issue() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM quote_versions v JOIN business_quotes q ON q.tenant_id=v.tenant_id AND q.quote_node_id=v.quote_node_id
        WHERE v.tenant_id=NEW.tenant_id AND v.quote_node_id=NEW.quote_node_id AND v.version=NEW.version
          AND q.current_version=v.version AND q.state='draft'
          AND EXISTS (SELECT 1 FROM events e WHERE e.tenant_id=NEW.tenant_id AND e.id=NEW.event_id AND e.type='quote.issued' AND e.actor_principal_id=NEW.issued_by_principal_id)
          AND ((v.digest_mode='r4-v1' AND EXISTS (SELECT 1 FROM quote_line_items l WHERE l.tenant_id=v.tenant_id AND l.quote_node_id=v.quote_node_id AND l.version=v.version)
                AND v.subtotal=(SELECT coalesce(sum(l.net_amount),0) FROM quote_line_items l WHERE l.tenant_id=v.tenant_id AND l.quote_node_id=v.quote_node_id AND l.version=v.version)
                AND v.tax_total=(SELECT coalesce(sum(round(l.net_amount*l.tax_rate,4)),0) FROM quote_line_items l WHERE l.tenant_id=v.tenant_id AND l.quote_node_id=v.quote_node_id AND l.version=v.version))
            OR (v.digest_mode='document-v1' AND EXISTS (SELECT 1 FROM quote_version_snapshots s WHERE s.tenant_id=v.tenant_id AND s.quote_node_id=v.quote_node_id AND s.version=v.version)
                AND EXISTS (SELECT 1 FROM quote_document_lines l WHERE l.tenant_id=v.tenant_id AND l.quote_node_id=v.quote_node_id AND l.version=v.version)
                AND v.subtotal=(SELECT coalesce(sum(l.net_amount),0) FROM quote_document_lines l WHERE l.tenant_id=v.tenant_id AND l.quote_node_id=v.quote_node_id AND l.version=v.version)
                AND v.tax_total=0))
    ) THEN RAISE EXCEPTION 'issue requires a complete current quote version'; END IF;
    RETURN NEW;
END;
$$;
