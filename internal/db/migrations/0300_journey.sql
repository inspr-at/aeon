-- SPDX-License-Identifier: AGPL-3.0-only
-- R3 journey projections. Canonical titles, keys, hierarchy and history stay in R1 nodes/events.
-- The requirement kind is installed when a project enters the journey, keeping
-- the R1/R2 tenant starter-kind set stable for tenants not using R3.
CREATE FUNCTION aeon_seed_requirement_kind(p_tenant_id uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    IF p_tenant_id IS DISTINCT FROM NULLIF(current_setting('aeon.tenant_id', true), '')::uuid THEN
        RAISE EXCEPTION 'tenant setting does not match requirement-kind tenant';
    END IF;
    INSERT INTO node_kinds(tenant_id, slug, label, short_prefix, icon, field_schema)
    VALUES (p_tenant_id, 'requirement', 'Requirement', 'REQ', 'requirement', '{}'::jsonb)
    ON CONFLICT (tenant_id, slug) DO NOTHING;
END;
$$;

CREATE TABLE journey_projects (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_node_id uuid NOT NULL,
    profile text NOT NULL DEFAULT 'personal' CHECK (profile IN ('personal', 'professional', 'enterprise')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    brief_confirmed_at timestamptz,
    decision text NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending', 'go', 'reduce_scope', 'park', 'drop')),
    requirements_revision bigint NOT NULL DEFAULT 0 CHECK (requirements_revision >= 0),
    agreed_requirements_revision bigint NOT NULL DEFAULT 0 CHECK (agreed_requirements_revision >= 0),
    agreed_requirements_digest_sha256 text CHECK (agreed_requirements_digest_sha256 ~ '^[0-9a-f]{64}$'),
    current_release_node_id uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, current_release_node_id) REFERENCES nodes(tenant_id, id),
    CHECK (agreed_requirements_revision <= requirements_revision),
    CHECK ((agreed_requirements_revision = 0) = (agreed_requirements_digest_sha256 IS NULL))
);
ALTER TABLE journey_projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_projects FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_projects_tenant ON journey_projects
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE journey_releases (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    release_node_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    number integer NOT NULL CHECK (number > 0),
    state text NOT NULL DEFAULT 'planning' CHECK (state IN
        ('planning', 'building', 'candidate', 'deploying', 'refused', 'access', 'released', 'superseded')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    access_required boolean NOT NULL DEFAULT false,
    version_scheme text CHECK (version_scheme IN ('legacy', 'inspr-calendar-v1', 'inspr-calendar-v2')),
    version text,
    released_at timestamptz,
    PRIMARY KEY (tenant_id, release_node_id),
    UNIQUE (tenant_id, project_node_id, release_node_id),
    UNIQUE (tenant_id, project_node_id, number),
    FOREIGN KEY (tenant_id, release_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id),
    CHECK ((version_scheme IS NULL) = (version IS NULL)),
    CHECK ((state IN ('released', 'superseded')) = (released_at IS NOT NULL))
);
CREATE INDEX journey_releases_project_idx ON journey_releases(tenant_id, project_node_id, number DESC);
ALTER TABLE journey_releases ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_releases FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_releases_tenant ON journey_releases
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
ALTER TABLE journey_projects ADD CONSTRAINT journey_current_release_fk
    FOREIGN KEY (tenant_id, project_node_id, current_release_node_id)
    REFERENCES journey_releases(tenant_id, project_node_id, release_node_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE journey_requirements (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    requirement_node_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('functional', 'nonfunctional')),
    revision bigint NOT NULL CHECK (revision > 0),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'agreed', 'superseded')),
    origin_draft_id uuid,
    creation_key text CHECK (creation_key IS NULL OR length(creation_key) BETWEEN 1 AND 128),
    PRIMARY KEY (tenant_id, requirement_node_id),
    UNIQUE (tenant_id, project_node_id, requirement_node_id),
    UNIQUE (tenant_id, project_node_id, creation_key),
    FOREIGN KEY (tenant_id, requirement_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id)
);
CREATE INDEX journey_requirements_project_idx ON journey_requirements(tenant_id, project_node_id, revision, kind);
ALTER TABLE journey_requirements ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_requirements FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_requirements_tenant ON journey_requirements
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- An agreed functional requirement becomes one R1 epic node, presented as a feature.
CREATE TABLE journey_features (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    feature_node_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    requirement_node_id uuid NOT NULL,
    PRIMARY KEY (tenant_id, feature_node_id),
    UNIQUE (tenant_id, project_node_id, feature_node_id),
    UNIQUE (tenant_id, requirement_node_id),
    FOREIGN KEY (tenant_id, feature_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, project_node_id, requirement_node_id)
        REFERENCES journey_requirements(tenant_id, project_node_id, requirement_node_id)
);
ALTER TABLE journey_features ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_features FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_features_tenant ON journey_features
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE journey_tickets (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    ticket_node_id uuid NOT NULL,
    project_node_id uuid NOT NULL,
    feature_node_id uuid,
    release_node_id uuid, -- NULL is the backlog.
    walker_position integer NOT NULL CHECK (walker_position >= 0),
    source text NOT NULL CHECK (source IN ('requirements', 'manual')),
    scope_revision_required boolean NOT NULL DEFAULT false,
    access_change boolean NOT NULL DEFAULT false,
    estimated_hours numeric(10, 2) CHECK (estimated_hours >= 0),
    PRIMARY KEY (tenant_id, ticket_node_id),
    UNIQUE (tenant_id, project_node_id, ticket_node_id),
    FOREIGN KEY (tenant_id, ticket_node_id) REFERENCES nodes(tenant_id, id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, project_node_id, feature_node_id)
        REFERENCES journey_features(tenant_id, project_node_id, feature_node_id),
    FOREIGN KEY (tenant_id, project_node_id, release_node_id)
        REFERENCES journey_releases(tenant_id, project_node_id, release_node_id)
);
CREATE INDEX journey_tickets_walk_idx ON journey_tickets(tenant_id, project_node_id, walker_position, ticket_node_id);
CREATE INDEX journey_tickets_release_idx ON journey_tickets(tenant_id, release_node_id, feature_node_id);
ALTER TABLE journey_tickets ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_tickets FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_tickets_tenant ON journey_tickets
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE journey_gates (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    project_node_id uuid NOT NULL,
    release_node_id uuid,
    gate text NOT NULL CHECK (gate IN ('shape', 'requirements', 'build', 'candidate', 'deploy', 'access')),
    approval_request_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, approval_request_id),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, project_node_id, release_node_id)
        REFERENCES journey_releases(tenant_id, project_node_id, release_node_id),
    FOREIGN KEY (tenant_id, approval_request_id) REFERENCES approval_requests(tenant_id, id),
    CHECK ((gate IN ('shape', 'requirements')) = (release_node_id IS NULL))
);
CREATE INDEX journey_gates_project_idx ON journey_gates(tenant_id, project_node_id, gate, created_at DESC);
ALTER TABLE journey_gates ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_gates FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_gates_tenant ON journey_gates
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE journey_action_receipts (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_node_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    resulting_revision bigint NOT NULL CHECK (resulting_revision > 0),
    event_id bigint NOT NULL,
    PRIMARY KEY (tenant_id, project_node_id, idempotency_key),
    FOREIGN KEY (tenant_id, project_node_id) REFERENCES journey_projects(tenant_id, project_node_id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, id)
);
ALTER TABLE journey_action_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE journey_action_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY journey_action_receipts_tenant ON journey_action_receipts
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

-- R1 node kinds remain tenant-configured, but these typed projections require their expected kinds.
CREATE FUNCTION aeon_guard_journey_node_kind() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_id uuid; expected_slug text;
BEGIN
    CASE TG_TABLE_NAME
        WHEN 'journey_projects' THEN target_id := NEW.project_node_id; expected_slug := 'project';
        WHEN 'journey_releases' THEN target_id := NEW.release_node_id; expected_slug := 'release';
        WHEN 'journey_requirements' THEN target_id := NEW.requirement_node_id; expected_slug := 'requirement';
        WHEN 'journey_features' THEN target_id := NEW.feature_node_id; expected_slug := 'epic';
        WHEN 'journey_tickets' THEN target_id := NEW.ticket_node_id; expected_slug := 'ticket';
    END CASE;
    IF NOT EXISTS (SELECT 1 FROM nodes n JOIN node_kinds k
        ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
        WHERE n.tenant_id = NEW.tenant_id AND n.id = target_id
          AND n.deleted_at IS NULL AND k.slug = expected_slug) THEN
        RAISE EXCEPTION '% requires a live % node', TG_TABLE_NAME, expected_slug;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER journey_projects_kind_guard BEFORE INSERT OR UPDATE OF project_node_id ON journey_projects
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_journey_node_kind();
CREATE TRIGGER journey_releases_kind_guard BEFORE INSERT OR UPDATE OF release_node_id ON journey_releases
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_journey_node_kind();
CREATE TRIGGER journey_requirements_kind_guard BEFORE INSERT OR UPDATE OF requirement_node_id ON journey_requirements
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_journey_node_kind();
CREATE TRIGGER journey_features_kind_guard BEFORE INSERT OR UPDATE OF feature_node_id ON journey_features
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_journey_node_kind();
CREATE TRIGGER journey_tickets_kind_guard BEFORE INSERT OR UPDATE OF ticket_node_id ON journey_tickets
    FOR EACH ROW EXECUTE FUNCTION aeon_guard_journey_node_kind();
