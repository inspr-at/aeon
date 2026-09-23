-- SPDX-License-Identifier: AGPL-3.0-only
-- Quote is an R1 node; each version and its lines are a frozen commercial offer.
CREATE TABLE business_quotes (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    customer_org_node_id uuid NOT NULL,
    current_version integer NOT NULL DEFAULT 0 CHECK (current_version >= 0),
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'issued', 'accepted', 'void')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, quote_node_id),
    FOREIGN KEY (tenant_id, quote_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, customer_org_node_id) REFERENCES nodes(tenant_id, id)
);
CREATE INDEX business_quotes_project_idx ON business_quotes(tenant_id, project_node_id, quote_node_id);
ALTER TABLE business_quotes ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_quotes FORCE ROW LEVEL SECURITY;
CREATE POLICY business_quotes_tenant ON business_quotes
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_business_quote() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT aeon_business_node_kind(NEW.tenant_id, NEW.quote_node_id, 'quote')
       OR NOT aeon_business_node_kind(NEW.tenant_id, NEW.project_node_id, 'project')
       OR NOT aeon_business_node_kind(NEW.tenant_id, NEW.customer_org_node_id, 'organisation') THEN
        RAISE EXCEPTION 'quote requires live quote, project and organisation nodes';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER business_quotes_kind_guard BEFORE INSERT OR UPDATE OF quote_node_id, project_node_id, customer_org_node_id
    ON business_quotes FOR EACH ROW EXECUTE FUNCTION aeon_guard_business_quote();

CREATE TABLE quote_versions (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    recipient_contact_node_id uuid NOT NULL,
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    terms_markdown text NOT NULL DEFAULT '',
    subtotal numeric(18, 4) NOT NULL CHECK (subtotal >= 0),
    tax_total numeric(18, 4) NOT NULL CHECK (tax_total >= 0),
    total numeric(18, 4) NOT NULL CHECK (total = subtotal + tax_total),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    created_by_principal_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, quote_node_id, version),
    FOREIGN KEY (tenant_id, quote_node_id) REFERENCES business_quotes(tenant_id, quote_node_id),
    FOREIGN KEY (tenant_id, recipient_contact_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE quote_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_versions_tenant ON quote_versions
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_quote_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT aeon_business_node_kind(NEW.tenant_id, NEW.recipient_contact_node_id, 'contact') THEN
        RAISE EXCEPTION 'quote recipient must be a live contact';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER quote_versions_contact_guard BEFORE INSERT ON quote_versions
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_version();
CREATE TRIGGER quote_versions_immutable BEFORE UPDATE OR DELETE ON quote_versions
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE quote_line_items (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    version integer NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    description text NOT NULL CHECK (length(btrim(description)) > 0),
    cost_unit_node_id uuid NOT NULL,
    unit text NOT NULL CHECK (unit IN ('hour', 'day', 'item')),
    quantity numeric(18, 4) NOT NULL CHECK (quantity > 0),
    rate_amount numeric(18, 4) NOT NULL CHECK (rate_amount >= 0),
    net_amount numeric(18, 4) NOT NULL CHECK (net_amount >= 0),
    tax_rate numeric(8, 5) NOT NULL DEFAULT 0 CHECK (tax_rate >= 0 AND tax_rate <= 1),
    PRIMARY KEY (tenant_id, quote_node_id, version, position),
    FOREIGN KEY (tenant_id, quote_node_id, version) REFERENCES quote_versions(tenant_id, quote_node_id, version),
    FOREIGN KEY (tenant_id, cost_unit_node_id) REFERENCES nodes(tenant_id, id),
    CHECK (net_amount = round(rate_amount * quantity, 4))
);
ALTER TABLE quote_line_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_line_items FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_line_items_tenant ON quote_line_items
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_quote_line() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT aeon_business_node_kind(NEW.tenant_id, NEW.cost_unit_node_id, 'cost_unit')
       OR EXISTS (SELECT 1 FROM quote_issues i WHERE i.tenant_id = NEW.tenant_id
                  AND i.quote_node_id = NEW.quote_node_id AND i.version = NEW.version) THEN
        RAISE EXCEPTION 'quote line requires a live cost unit and unissued version';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TABLE quote_issues (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    version integer NOT NULL,
    issued_by_principal_id uuid NOT NULL,
    issued_at timestamptz NOT NULL DEFAULT now(),
    event_id bigint NOT NULL,
    PRIMARY KEY (tenant_id, quote_node_id, version),
    UNIQUE (tenant_id, event_id),
    FOREIGN KEY (tenant_id, quote_node_id, version) REFERENCES quote_versions(tenant_id, quote_node_id, version),
    FOREIGN KEY (tenant_id, issued_by_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, id)
);
ALTER TABLE quote_issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_issues FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_issues_tenant ON quote_issues
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_quote_issue() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM quote_versions v JOIN business_quotes q
          ON q.tenant_id = v.tenant_id AND q.quote_node_id = v.quote_node_id
        WHERE v.tenant_id = NEW.tenant_id AND v.quote_node_id = NEW.quote_node_id
          AND v.version = NEW.version AND q.current_version = v.version
          AND q.state = 'draft'
          AND EXISTS (SELECT 1 FROM events e WHERE e.tenant_id = NEW.tenant_id
                      AND e.id = NEW.event_id AND e.type = 'quote.issued'
                      AND e.actor_principal_id = NEW.issued_by_principal_id)
          AND EXISTS (SELECT 1 FROM quote_line_items l WHERE l.tenant_id = v.tenant_id
                      AND l.quote_node_id = v.quote_node_id AND l.version = v.version)
          AND v.subtotal = (SELECT coalesce(sum(l.net_amount), 0) FROM quote_line_items l
                            WHERE l.tenant_id = v.tenant_id AND l.quote_node_id = v.quote_node_id
                              AND l.version = v.version)
          AND v.tax_total = (SELECT coalesce(sum(round(l.net_amount * l.tax_rate, 4)), 0)
                             FROM quote_line_items l WHERE l.tenant_id = v.tenant_id
                               AND l.quote_node_id = v.quote_node_id AND l.version = v.version)
    ) THEN
        RAISE EXCEPTION 'issue requires a complete current quote version';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER quote_issues_guard BEFORE INSERT ON quote_issues
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_issue();
CREATE TRIGGER quote_line_items_guard BEFORE INSERT ON quote_line_items
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_line();
CREATE TRIGGER quote_line_items_immutable BEFORE UPDATE OR DELETE ON quote_line_items
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER quote_issues_immutable BEFORE UPDATE OR DELETE ON quote_issues
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();

CREATE TABLE quote_acceptances (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quote_node_id uuid NOT NULL,
    version integer NOT NULL,
    customer_principal_id uuid NOT NULL,
    recipient_contact_node_id uuid NOT NULL,
    accepted_content_sha256 text NOT NULL CHECK (accepted_content_sha256 ~ '^[0-9a-f]{64}$'),
    accepted_at timestamptz NOT NULL DEFAULT now(),
    event_id bigint NOT NULL,
    PRIMARY KEY (tenant_id, quote_node_id, version),
    UNIQUE (tenant_id, event_id),
    FOREIGN KEY (tenant_id, quote_node_id, version) REFERENCES quote_issues(tenant_id, quote_node_id, version),
    FOREIGN KEY (tenant_id, recipient_contact_node_id, customer_principal_id)
        REFERENCES crm_contact_principals(tenant_id, contact_node_id, principal_id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, id)
);
ALTER TABLE quote_acceptances ENABLE ROW LEVEL SECURITY;
ALTER TABLE quote_acceptances FORCE ROW LEVEL SECURITY;
CREATE POLICY quote_acceptances_tenant ON quote_acceptances
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
CREATE FUNCTION aeon_guard_quote_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM quote_versions v JOIN business_quotes q
          ON q.tenant_id = v.tenant_id AND q.quote_node_id = v.quote_node_id
        JOIN principals p ON p.tenant_id = NEW.tenant_id AND p.id = NEW.customer_principal_id
        WHERE v.tenant_id = NEW.tenant_id AND v.quote_node_id = NEW.quote_node_id
          AND v.version = NEW.version AND q.current_version = v.version
          AND q.state = 'issued' AND p.kind = 'person'
          AND v.recipient_contact_node_id = NEW.recipient_contact_node_id
          AND v.content_sha256 = NEW.accepted_content_sha256
          AND EXISTS (SELECT 1 FROM events e WHERE e.tenant_id = NEW.tenant_id
                      AND e.id = NEW.event_id AND e.type = 'quote.accepted'
                      AND e.actor_principal_id = NEW.customer_principal_id)
          AND EXISTS (
              SELECT 1 FROM node_relations r WHERE r.tenant_id = NEW.tenant_id
                AND r.type = 'contact_for'
                AND r.source_node_id = NEW.recipient_contact_node_id
                AND r.target_node_id = q.customer_org_node_id
          )
    ) THEN
        RAISE EXCEPTION 'acceptance requires the current issued version, recipient and digest';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER quote_acceptances_guard BEFORE INSERT ON quote_acceptances
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_quote_acceptance();
CREATE TRIGGER quote_acceptances_immutable BEFORE UPDATE OR DELETE ON quote_acceptances
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
