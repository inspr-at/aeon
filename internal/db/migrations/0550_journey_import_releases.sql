-- SPDX-License-Identifier: AGPL-3.0-only
-- Register imported release nodes that had no classic membership relation.
CREATE FUNCTION aeon_backfill_journey_releases(p_tenant uuid) RETURNS integer
LANGUAGE plpgsql AS $$
DECLARE rec record; project_inserted uuid; release_inserted uuid; next_number integer; changed integer := 0;
BEGIN
  IF p_tenant IS DISTINCT FROM NULLIF(current_setting('aeon.tenant_id', true), '')::uuid THEN
    RAISE EXCEPTION 'tenant setting does not match journey backfill tenant';
  END IF;
  FOR rec IN
    WITH RECURSIVE ancestors AS (
      SELECT n.id AS release_id,n.id,n.parent_id,0 AS depth FROM nodes n
      JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
      WHERE n.tenant_id=p_tenant AND k.slug='release' AND n.deleted_at IS NULL
        AND EXISTS(SELECT 1 FROM events e WHERE e.tenant_id=n.tenant_id AND e.node_id=n.id AND e.type='import.node_created')
        AND NOT EXISTS(SELECT 1 FROM journey_releases r WHERE r.tenant_id=n.tenant_id AND r.release_node_id=n.id)
      UNION ALL
      SELECT a.release_id,n.id,n.parent_id,a.depth+1 FROM ancestors a
      JOIN nodes n ON n.tenant_id=p_tenant AND n.id=a.parent_id WHERE a.depth<64
    )
    SELECT release.id AS release_id,
      coalesce((SELECT a.id FROM ancestors a JOIN nodes pn ON pn.tenant_id=p_tenant AND pn.id=a.id
        JOIN node_kinds pk ON pk.tenant_id=p_tenant AND pk.id=pn.kind_id
        WHERE a.release_id=release.id AND pk.slug='project' AND pn.deleted_at IS NULL ORDER BY a.depth LIMIT 1),
        (SELECT pn.id FROM nodes pn JOIN node_kinds pk ON pk.tenant_id=pn.tenant_id AND pk.id=pn.kind_id
        WHERE pn.tenant_id=p_tenant AND pk.slug='project' AND pn.deleted_at IS NULL
          AND pn.fields->'classic'->>'id'=release.fields->'classic'->>'project_id' LIMIT 1)) AS project_id,
      (SELECT e.actor_principal_id FROM events e WHERE e.tenant_id=p_tenant AND e.node_id=release.id
        AND e.type='import.node_created' ORDER BY e.id LIMIT 1) AS actor_id
    FROM nodes release JOIN node_kinds rk ON rk.tenant_id=release.tenant_id AND rk.id=release.kind_id
    WHERE release.tenant_id=p_tenant AND rk.slug='release' AND release.deleted_at IS NULL
      AND EXISTS(SELECT 1 FROM events e WHERE e.tenant_id=p_tenant AND e.node_id=release.id AND e.type='import.node_created')
      AND NOT EXISTS(SELECT 1 FROM journey_releases r WHERE r.tenant_id=p_tenant AND r.release_node_id=release.id)
    ORDER BY release.created_at,release.id
  LOOP
    IF rec.project_id IS NULL OR rec.actor_id IS NULL THEN CONTINUE; END IF;
    PERFORM aeon_seed_requirement_kind(p_tenant);
    INSERT INTO journey_projects(tenant_id,project_node_id) VALUES(p_tenant,rec.project_id)
      ON CONFLICT DO NOTHING RETURNING project_node_id INTO project_inserted;
    IF project_inserted IS NOT NULL THEN
      INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after)
        VALUES(p_tenant,rec.actor_id,rec.project_id,'journey.import_project_registered',
          jsonb_build_object('project_node_id',rec.project_id));
    END IF;
    project_inserted := NULL;
    PERFORM 1 FROM journey_projects WHERE tenant_id=p_tenant AND project_node_id=rec.project_id FOR UPDATE;
    SELECT coalesce(max(number),0)+1 INTO next_number FROM journey_releases
      WHERE tenant_id=p_tenant AND project_node_id=rec.project_id;
    INSERT INTO journey_releases(tenant_id,release_node_id,project_node_id,number,state)
      VALUES(p_tenant,rec.release_id,rec.project_id,next_number,'planning')
      ON CONFLICT DO NOTHING RETURNING release_node_id INTO release_inserted;
    IF release_inserted IS NOT NULL THEN
      INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after)
        VALUES(p_tenant,rec.actor_id,rec.release_id,'journey.import_release_registered',
          jsonb_build_object('project_node_id',rec.project_id,'release_node_id',rec.release_id,'number',next_number));
      changed := changed + 1;
    END IF;
    release_inserted := NULL;
  END LOOP;
  RETURN changed;
END;
$$;

DO $$ DECLARE tenant_row record; BEGIN
  FOR tenant_row IN SELECT id FROM tenants LOOP
    PERFORM set_config('aeon.tenant_id',tenant_row.id::text,true);
    PERFORM aeon_backfill_journey_releases(tenant_row.id);
  END LOOP;
END $$;
