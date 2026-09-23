-- SPDX-License-Identifier: AGPL-3.0-only
-- German/English weighted lexical search and asynchronously maintained vectors.
CREATE EXTENSION IF NOT EXISTS vector;
ALTER TABLE nodes ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('german', title), 'A') ||
    setweight(to_tsvector('english', title), 'A') ||
    setweight(to_tsvector('german', body), 'B') ||
    setweight(to_tsvector('english', body), 'B')
) STORED;
CREATE INDEX nodes_search_document_idx ON nodes USING gin(search_document) WHERE deleted_at IS NULL;

CREATE TABLE node_embeddings (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    node_id uuid NOT NULL,
    model text NOT NULL CHECK (length(btrim(model)) > 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    embedding halfvec(1536) NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, node_id),
    FOREIGN KEY (tenant_id, node_id) REFERENCES nodes(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX node_embeddings_hnsw_idx ON node_embeddings USING hnsw (embedding halfvec_cosine_ops);
ALTER TABLE node_embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE node_embeddings FORCE ROW LEVEL SECURITY;
CREATE POLICY node_embeddings_tenant ON node_embeddings
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE TABLE node_embedding_jobs (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    node_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text,
    queued_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, node_id),
    FOREIGN KEY (tenant_id, node_id) REFERENCES nodes(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX node_embedding_jobs_queue_idx ON node_embedding_jobs(tenant_id, queued_at, node_id)
    WHERE status IN ('queued', 'failed');
ALTER TABLE node_embedding_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE node_embedding_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY node_embedding_jobs_tenant ON node_embedding_jobs
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);

CREATE FUNCTION aeon_queue_node_embedding() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO node_embedding_jobs(tenant_id, node_id, status, attempts, last_error, queued_at, updated_at)
    VALUES (NEW.tenant_id, NEW.id, 'queued', 0, NULL, now(), now())
    ON CONFLICT (tenant_id, node_id) DO UPDATE
        SET status = 'queued', attempts = 0, last_error = NULL,
            queued_at = now(), updated_at = now();
    RETURN NEW;
END;
$$;
CREATE TRIGGER nodes_embedding_enqueue
AFTER INSERT OR UPDATE OF title, body ON nodes
FOR EACH ROW EXECUTE FUNCTION aeon_queue_node_embedding();

-- Both ranked candidate sets are fused in one SQL statement. The caller supplies
-- a query embedding from the same model as stored vectors, or NULL for lexical-only.
-- API cursor pagination is applied by the search module to (score, node_id).
CREATE FUNCTION aeon_search_nodes(
    p_query text,
    p_embedding halfvec(1536),
    p_model text,
    p_kind_id uuid DEFAULT NULL,
    p_state text DEFAULT NULL,
    p_limit integer DEFAULT 50
) RETURNS TABLE(node_id uuid, score double precision)
LANGUAGE sql STABLE AS $$
    WITH terms AS (
        SELECT websearch_to_tsquery('german', p_query) ||
               websearch_to_tsquery('english', p_query) AS q
    ),
    lexical_candidates AS (
        SELECT n.id, ts_rank_cd(n.search_document, t.q) AS lexical_score
        FROM nodes n CROSS JOIN terms t
        WHERE n.tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
          AND n.deleted_at IS NULL
          AND (p_kind_id IS NULL OR n.kind_id = p_kind_id)
          AND (p_state IS NULL OR n.state = p_state)
          AND n.search_document @@ t.q
        ORDER BY lexical_score DESC, n.id
        LIMIT 1000
    ),
    lexical AS (
        SELECT id, row_number() OVER (ORDER BY lexical_score DESC, id) AS rank
        FROM lexical_candidates
    ),
    semantic_candidates AS (
        SELECT n.id, e.embedding <=> p_embedding AS distance
        FROM node_embeddings e
        JOIN nodes n ON n.tenant_id = e.tenant_id AND n.id = e.node_id
        WHERE p_embedding IS NOT NULL
          AND e.tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
          AND e.model = p_model
          AND n.deleted_at IS NULL
          AND NOT EXISTS (
              SELECT 1 FROM node_embedding_jobs j
              WHERE j.tenant_id = e.tenant_id AND j.node_id = e.node_id
          )
          AND (p_kind_id IS NULL OR n.kind_id = p_kind_id)
          AND (p_state IS NULL OR n.state = p_state)
        ORDER BY e.embedding <=> p_embedding, n.id
        LIMIT 1000
    ),
    semantic AS (
        SELECT id, row_number() OVER (ORDER BY distance, id) AS rank
        FROM semantic_candidates
    ),
    fused AS (
        SELECT coalesce(l.id, s.id) AS id,
               coalesce(1.0 / (60 + l.rank), 0) +
               coalesce(1.0 / (60 + s.rank), 0) AS rrf_score
        FROM lexical l FULL JOIN semantic s USING (id)
    )
    SELECT f.id, f.rrf_score::double precision
    FROM fused f
    ORDER BY f.rrf_score DESC, f.id
    LIMIT least(greatest(p_limit, 1), 200);
$$;
