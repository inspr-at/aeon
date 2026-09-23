-- SPDX-License-Identifier: AGPL-3.0-only
-- Aithema intake preserves source provenance and immutable proposals. Acceptance is separate.
CREATE TABLE intake_sources (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    project_node_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('url', 'file', 'note', 'conversation')),
    label text NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 512),
    locator text CHECK (locator IS NULL OR length(locator) <= 2048),
    file_id uuid,
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    created_by_principal_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, project_node_id, id),
    UNIQUE (tenant_id, project_node_id, idempotency_key),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, created_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK ((kind = 'file') = (file_id IS NOT NULL))
);
CREATE INDEX intake_sources_project_idx ON intake_sources(tenant_id, project_node_id, created_at, id);
ALTER TABLE intake_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE intake_sources FORCE ROW LEVEL SECURITY;
CREATE POLICY intake_sources_tenant ON intake_sources
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE intake_transcript_turns (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    project_node_id uuid NOT NULL,
    source_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    speaker text NOT NULL CHECK (speaker IN ('person', 'agent')),
    speaker_principal_id uuid,
    body text NOT NULL CHECK (length(body) BETWEEN 1 AND 65536),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, project_node_id, id),
    UNIQUE (tenant_id, source_id, ordinal),
    UNIQUE (tenant_id, project_node_id, idempotency_key),
    FOREIGN KEY (tenant_id, project_node_id, source_id)
        REFERENCES intake_sources(tenant_id, project_node_id, id),
    FOREIGN KEY (tenant_id, speaker_principal_id) REFERENCES principals(tenant_id, id)
);
ALTER TABLE intake_transcript_turns ENABLE ROW LEVEL SECURITY;
ALTER TABLE intake_transcript_turns FORCE ROW LEVEL SECURITY;
CREATE POLICY intake_transcript_turns_tenant ON intake_transcript_turns
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE intake_drafts (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    project_node_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('brief', 'requirement')),
    requirement_kind text CHECK (requirement_kind IN ('functional', 'nonfunctional')),
    target_node_id uuid,
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    body text NOT NULL,
    base_event_id bigint NOT NULL DEFAULT 0 CHECK (base_event_id >= 0),
    proposed_by_principal_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    proposed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, project_node_id, id),
    UNIQUE (tenant_id, project_node_id, idempotency_key),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, target_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, proposed_by_principal_id) REFERENCES principals(tenant_id, id),
    CHECK ((kind = 'requirement') = (requirement_kind IS NOT NULL)),
    CHECK ((target_node_id IS NULL) = (base_event_id = 0))
);
CREATE INDEX intake_drafts_project_idx ON intake_drafts(tenant_id, project_node_id, proposed_at DESC, id);
ALTER TABLE intake_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE intake_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY intake_drafts_tenant ON intake_drafts
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
ALTER TABLE journey_requirements ADD CONSTRAINT journey_requirement_origin_draft_fk
    FOREIGN KEY (tenant_id, project_node_id, origin_draft_id)
    REFERENCES intake_drafts(tenant_id, project_node_id, id);

CREATE TABLE intake_citations (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    draft_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    source_id uuid NOT NULL,
    turn_id uuid,
    locator text NOT NULL CHECK (length(locator) BETWEEN 1 AND 256),
    quote_sha256 text CHECK (quote_sha256 ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY (tenant_id, draft_id, ordinal),
    FOREIGN KEY (tenant_id, project_node_id, draft_id)
        REFERENCES intake_drafts(tenant_id, project_node_id, id),
    FOREIGN KEY (tenant_id, project_node_id, source_id)
        REFERENCES intake_sources(tenant_id, project_node_id, id),
    FOREIGN KEY (tenant_id, project_node_id, turn_id)
        REFERENCES intake_transcript_turns(tenant_id, project_node_id, id)
);
ALTER TABLE intake_citations ENABLE ROW LEVEL SECURITY;
ALTER TABLE intake_citations FORCE ROW LEVEL SECURITY;
CREATE POLICY intake_citations_tenant ON intake_citations
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- Accepted suggestions are the only Aithema-generated ticket inputs. A draft
-- may have none; an empty breakdown never fabricates a ticket.
CREATE TABLE intake_draft_ticket_suggestions (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    draft_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    estimated_hours numeric(10, 2) NOT NULL CHECK (estimated_hours >= 0),
    later boolean NOT NULL DEFAULT false,
    access_change boolean NOT NULL DEFAULT false,
    PRIMARY KEY (tenant_id, draft_id, ordinal),
    FOREIGN KEY (tenant_id, project_node_id, draft_id)
        REFERENCES intake_drafts(tenant_id, project_node_id, id)
);
ALTER TABLE intake_draft_ticket_suggestions ENABLE ROW LEVEL SECURITY;
ALTER TABLE intake_draft_ticket_suggestions FORCE ROW LEVEL SECURITY;
CREATE POLICY intake_draft_ticket_suggestions_tenant ON intake_draft_ticket_suggestions
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE intake_draft_acceptances (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    draft_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    accepted_by_principal_id uuid NOT NULL,
    target_node_id uuid NOT NULL,
    event_id bigint NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, draft_id),
    FOREIGN KEY (tenant_id, project_node_id, draft_id)
        REFERENCES intake_drafts(tenant_id, project_node_id, id),
    FOREIGN KEY (tenant_id, accepted_by_principal_id) REFERENCES principals(tenant_id, id),
    FOREIGN KEY (tenant_id, target_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, id)
);
ALTER TABLE intake_draft_acceptances ENABLE ROW LEVEL SECURITY;
ALTER TABLE intake_draft_acceptances FORCE ROW LEVEL SECURITY;
CREATE POLICY intake_draft_acceptances_tenant ON intake_draft_acceptances
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TRIGGER intake_sources_immutable BEFORE UPDATE OR DELETE ON intake_sources
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER intake_turns_immutable BEFORE UPDATE OR DELETE ON intake_transcript_turns
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER intake_drafts_immutable BEFORE UPDATE OR DELETE ON intake_drafts
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER intake_citations_immutable BEFORE UPDATE OR DELETE ON intake_citations
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER intake_ticket_suggestions_immutable BEFORE UPDATE OR DELETE ON intake_draft_ticket_suggestions
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
CREATE TRIGGER intake_acceptances_immutable BEFORE UPDATE OR DELETE ON intake_draft_acceptances
    FOR EACH ROW EXECUTE FUNCTION aeon_events_append_only();
