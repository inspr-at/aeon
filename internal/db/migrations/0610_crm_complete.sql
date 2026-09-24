-- SPDX-License-Identifier: AGPL-3.0-only
-- Customer/contact details live on R1 nodes; these projections hold CRM-only
-- concurrency and membership invariants. No tenant content is seeded here.
CREATE TABLE crm_organisation_profiles (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 organisation_node_id uuid NOT NULL,
 primary_contact_node_id uuid,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 PRIMARY KEY (tenant_id, organisation_node_id),
 FOREIGN KEY (tenant_id, organisation_node_id) REFERENCES nodes(tenant_id,id),
 FOREIGN KEY (tenant_id, primary_contact_node_id) REFERENCES nodes(tenant_id,id)
);
CREATE TABLE crm_contact_profiles (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 contact_node_id uuid NOT NULL,
 organisation_node_id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 PRIMARY KEY (tenant_id, contact_node_id),
 FOREIGN KEY (tenant_id, contact_node_id) REFERENCES nodes(tenant_id,id),
 FOREIGN KEY (tenant_id, organisation_node_id) REFERENCES nodes(tenant_id,id)
);
CREATE INDEX crm_contact_profiles_org_idx ON crm_contact_profiles(tenant_id,organisation_node_id,contact_node_id);
ALTER TABLE crm_organisation_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_organisation_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_organisation_profiles_tenant ON crm_organisation_profiles USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
ALTER TABLE crm_contact_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_contact_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_contact_profiles_tenant ON crm_contact_profiles USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE FUNCTION aeon_guard_crm_primary() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT aeon_business_node_kind(NEW.tenant_id,NEW.organisation_node_id,'organisation') THEN RAISE EXCEPTION 'CRM profile needs live organisation'; END IF;
 IF NEW.primary_contact_node_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM crm_contact_profiles c JOIN nodes n ON n.tenant_id=c.tenant_id AND n.id=c.contact_node_id
  WHERE c.tenant_id=NEW.tenant_id AND c.contact_node_id=NEW.primary_contact_node_id
    AND c.organisation_node_id=NEW.organisation_node_id AND n.deleted_at IS NULL
 ) THEN RAISE EXCEPTION 'primary contact must belong to customer'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_primary_guard BEFORE INSERT OR UPDATE ON crm_organisation_profiles FOR EACH ROW EXECUTE FUNCTION aeon_guard_crm_primary();
CREATE FUNCTION aeon_guard_crm_contact_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT aeon_business_node_kind(NEW.tenant_id,NEW.contact_node_id,'contact')
    OR NOT aeon_business_node_kind(NEW.tenant_id,NEW.organisation_node_id,'organisation') THEN RAISE EXCEPTION 'CRM contact needs live contact and customer'; END IF;
 IF TG_OP='UPDATE' AND NEW.organisation_node_id IS DISTINCT FROM OLD.organisation_node_id THEN RAISE EXCEPTION 'CRM contact customer is immutable'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_contact_profile_guard BEFORE INSERT OR UPDATE ON crm_contact_profiles FOR EACH ROW EXECUTE FUNCTION aeon_guard_crm_contact_profile();

CREATE TABLE crm_document_metadata (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 attachment_id uuid NOT NULL,
 title text NOT NULL CHECK (length(title) <= 255),
 category text NOT NULL CHECK (length(category) <= 100),
 status text NOT NULL CHECK (status IN ('draft','active','expired')),
 valid_from date,
 valid_until date,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 PRIMARY KEY (tenant_id,attachment_id),
 FOREIGN KEY (tenant_id,attachment_id) REFERENCES attachments(tenant_id,id),
 CHECK (valid_from IS NULL OR valid_until IS NULL OR valid_until >= valid_from)
);
ALTER TABLE crm_document_metadata ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_document_metadata FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_document_metadata_tenant ON crm_document_metadata USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE crm_project_cooperation (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 project_node_id uuid NOT NULL,
 data jsonb NOT NULL CHECK (jsonb_typeof(data)='object'),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 PRIMARY KEY (tenant_id,project_node_id),
 FOREIGN KEY (tenant_id,project_node_id) REFERENCES nodes(tenant_id,id)
);
ALTER TABLE crm_project_cooperation ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_project_cooperation FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_project_cooperation_tenant ON crm_project_cooperation USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE crm_provider_configs (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 provider_id text NOT NULL CHECK (provider_id ~ '^[a-z][a-z0-9_]*$'),
 enabled boolean NOT NULL DEFAULT false,
 secret_ref text NOT NULL DEFAULT '' CHECK (length(secret_ref) <= 500),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 PRIMARY KEY (tenant_id,provider_id)
);
ALTER TABLE crm_provider_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_provider_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_provider_configs_tenant ON crm_provider_configs USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
CREATE TABLE crm_provider_config_history (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 event_id bigint NOT NULL,
 provider_id text NOT NULL,
 previous_enabled boolean,
 previous_secret_ref text,
 previous_revision bigint,
 PRIMARY KEY (tenant_id,event_id),
 FOREIGN KEY (tenant_id,event_id) REFERENCES events(tenant_id,id)
);
ALTER TABLE crm_provider_config_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_provider_config_history FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_provider_config_history_tenant ON crm_provider_config_history USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

CREATE TABLE crm_note_rewrite_drafts (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 id uuid NOT NULL DEFAULT gen_random_uuid(),
 organisation_node_id uuid NOT NULL,
 base_revision bigint NOT NULL CHECK (base_revision > 0),
 proposed_text text NOT NULL CHECK (length(proposed_text) BETWEEN 1 AND 20000),
 proposed_by_principal_id uuid NOT NULL,
 proposed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 applied_at timestamptz,
 PRIMARY KEY (tenant_id,id),
 FOREIGN KEY (tenant_id,organisation_node_id) REFERENCES nodes(tenant_id,id),
 FOREIGN KEY (tenant_id,proposed_by_principal_id) REFERENCES principals(tenant_id,id)
);
ALTER TABLE crm_note_rewrite_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_note_rewrite_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_note_rewrite_drafts_tenant ON crm_note_rewrite_drafts USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);

-- Existing tenant kind schemas gain optional CRM fields without replacing
-- tenant custom properties or required lists. New plugin manifests carry the
-- same field vocabulary for newly installed tenants.
UPDATE node_kinds SET field_schema=jsonb_set(
 jsonb_set(field_schema,'{properties}',
  (CASE WHEN slug='organisation' THEN
   '{"legal_name":{"type":"string"},"industry":{"type":"string"},"website":{"type":"string"},"domain":{"type":"string"},"phone":{"type":"string"},"description":{"type":"string"},"customer_notes":{"type":"string"},"vat_id":{"type":"string"},"tax_id":{"type":"string"},"register_no":{"type":"string"},"employee_count":{"type":["integer","null"],"minimum":0},"annual_revenue_minor":{"type":["integer","null"],"minimum":0},"currency":{"type":"string"},"billing_address":{"type":["object","null"]},"visiting_address":{"type":["object","null"]},"hourly_rate_minor":{"type":["integer","null"],"minimum":0},"lp_rate_minor":{"type":["integer","null"],"minimum":0},"external_provider":{"type":"string"},"external_id":{"type":"string"},"external_url":{"type":"string"}}'::jsonb
  ELSE '{"email":{"type":"string"},"phone":{"type":"string"},"role":{"type":"string"},"note":{"type":"string"},"external_provider":{"type":"string"},"external_id":{"type":"string"},"external_url":{"type":"string"}}'::jsonb END)
  || coalesce(field_schema->'properties','{}'::jsonb), true),
 '{type}','"object"'::jsonb,true), updated_at=clock_timestamp()
WHERE slug IN ('organisation','contact');

INSERT INTO crm_organisation_profiles(tenant_id,organisation_node_id)
SELECT n.tenant_id,n.id FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
WHERE k.slug='organisation' AND n.deleted_at IS NULL;
INSERT INTO crm_contact_profiles(tenant_id,contact_node_id,organisation_node_id)
SELECT n.tenant_id,n.id,links.target_node_id FROM nodes n
JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
JOIN LATERAL (SELECT r.target_node_id FROM node_relations r JOIN nodes o ON o.tenant_id=r.tenant_id AND o.id=r.target_node_id
 JOIN node_kinds ok ON ok.tenant_id=o.tenant_id AND ok.id=o.kind_id
 WHERE r.tenant_id=n.tenant_id AND r.source_node_id=n.id AND r.type='contact_for' AND ok.slug='organisation' AND o.deleted_at IS NULL
 ORDER BY r.id LIMIT 1) links ON true
WHERE k.slug='contact' AND n.deleted_at IS NULL;
UPDATE crm_organisation_profiles o SET primary_contact_node_id=(
 SELECT c.contact_node_id FROM crm_contact_profiles c WHERE c.tenant_id=o.tenant_id AND c.organisation_node_id=o.organisation_node_id
 ORDER BY c.contact_node_id LIMIT 1);

-- Generic node/relationship endpoints may still be used. Prevent them from
-- bypassing the domain deletion/primary invariants.
CREATE FUNCTION aeon_guard_crm_node_delete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE slug text;
BEGIN
 IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
  SELECT k.slug INTO slug FROM node_kinds k WHERE k.tenant_id=NEW.tenant_id AND k.id=NEW.kind_id;
  IF slug='organisation' AND (
    EXISTS(SELECT 1 FROM business_quotes q WHERE q.tenant_id=NEW.tenant_id AND q.customer_org_node_id=NEW.id)
    OR EXISTS(SELECT 1 FROM node_relations r JOIN nodes p ON p.tenant_id=r.tenant_id AND p.id=r.target_node_id WHERE r.tenant_id=NEW.tenant_id AND r.source_node_id=NEW.id AND r.type='customer_of' AND p.deleted_at IS NULL AND EXISTS(SELECT 1 FROM node_kinds k WHERE k.tenant_id=p.tenant_id AND k.id=p.kind_id AND k.slug='project'))
    OR EXISTS(SELECT 1 FROM crm_contact_profiles c JOIN nodes n ON n.tenant_id=c.tenant_id AND n.id=c.contact_node_id WHERE c.tenant_id=NEW.tenant_id AND c.organisation_node_id=NEW.id AND n.deleted_at IS NULL)
  ) THEN RAISE EXCEPTION 'customer has contacts, projects or quotes'; END IF;
  IF slug='contact' AND EXISTS(SELECT 1 FROM crm_organisation_profiles o WHERE o.tenant_id=NEW.tenant_id AND o.primary_contact_node_id=NEW.id) THEN RAISE EXCEPTION 'promote successor before deleting primary contact'; END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_node_delete_guard BEFORE UPDATE OF deleted_at ON nodes FOR EACH ROW EXECUTE FUNCTION aeon_guard_crm_node_delete();
CREATE UNIQUE INDEX crm_one_customer_per_target ON node_relations(tenant_id,target_node_id) WHERE type='customer_of';
CREATE FUNCTION aeon_guard_crm_relation_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.type='contact_for' AND EXISTS(SELECT 1 FROM crm_contact_profiles c WHERE c.tenant_id=OLD.tenant_id AND c.contact_node_id=OLD.source_node_id AND c.organisation_node_id=OLD.target_node_id) THEN
  RAISE EXCEPTION 'managed customer contact link cannot be removed directly';
 END IF;
 IF OLD.type='customer_of' AND EXISTS(SELECT 1 FROM business_quotes q WHERE q.tenant_id=OLD.tenant_id AND q.project_node_id=OLD.target_node_id AND q.customer_org_node_id=OLD.source_node_id) THEN
  RAISE EXCEPTION 'project customer link is used by quote';
 END IF;
 RETURN OLD;
END; $$;
CREATE TRIGGER crm_relation_delete_guard BEFORE DELETE ON node_relations FOR EACH ROW EXECUTE FUNCTION aeon_guard_crm_relation_delete();
CREATE OR REPLACE FUNCTION aeon_guard_customer_number() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT aeon_business_node_kind(NEW.tenant_id,NEW.organisation_node_id,'organisation') THEN RAISE EXCEPTION 'customer number requires live organisation'; END IF;
 IF TG_OP='UPDATE' THEN
  IF NEW.organisation_node_id IS DISTINCT FROM OLD.organisation_node_id
     OR EXISTS(SELECT 1 FROM business_quotes q WHERE q.tenant_id=NEW.tenant_id AND q.customer_org_node_id=NEW.organisation_node_id AND q.state<>'draft')
     OR NOT ((OLD.customer_no ~ '^K[0-9]{2}-[0-9]{3,}$' AND NEW.customer_no ~ '^K[0-9]{4}[1-9][0-9]*$' AND NEW.provenance='converted')
       OR (current_setting('aeon.crm_undo',true)='on' AND OLD.provenance='converted' AND NEW.customer_no ~ '^K[0-9]{2}-[0-9]{3,}$' AND NEW.provenance='imported')) THEN
   RAISE EXCEPTION 'customer number conversion requires legacy number and draft-only quotes';
  END IF;
 END IF;
 RETURN NEW;
END; $$;

CREATE FUNCTION aeon_seed_crm_node_profile() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE slug text;
BEGIN
 SELECT k.slug INTO slug FROM node_kinds k WHERE k.tenant_id=NEW.tenant_id AND k.id=NEW.kind_id;
 IF slug='organisation' AND NEW.deleted_at IS NULL THEN
  INSERT INTO crm_organisation_profiles(tenant_id,organisation_node_id) VALUES(NEW.tenant_id,NEW.id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_seed_node_profile AFTER INSERT ON nodes FOR EACH ROW EXECUTE FUNCTION aeon_seed_crm_node_profile();
CREATE FUNCTION aeon_seed_crm_contact_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.type='contact_for' AND aeon_business_node_kind(NEW.tenant_id,NEW.source_node_id,'contact') AND aeon_business_node_kind(NEW.tenant_id,NEW.target_node_id,'organisation') THEN
  INSERT INTO crm_contact_profiles(tenant_id,contact_node_id,organisation_node_id)
  VALUES(NEW.tenant_id,NEW.source_node_id,NEW.target_node_id)
  ON CONFLICT(tenant_id,contact_node_id) DO UPDATE SET organisation_node_id=EXCLUDED.organisation_node_id;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_seed_contact_profile AFTER INSERT ON node_relations FOR EACH ROW EXECUTE FUNCTION aeon_seed_crm_contact_profile();
CREATE FUNCTION aeon_bump_crm_node_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.title IS DISTINCT FROM OLD.title OR NEW.fields IS DISTINCT FROM OLD.fields THEN
  UPDATE crm_organisation_profiles SET revision=revision+1 WHERE tenant_id=NEW.tenant_id AND organisation_node_id=NEW.id;
  UPDATE crm_contact_profiles SET revision=revision+1 WHERE tenant_id=NEW.tenant_id AND contact_node_id=NEW.id;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_bump_node_revision AFTER UPDATE OF title,fields ON nodes FOR EACH ROW EXECUTE FUNCTION aeon_bump_crm_node_revision();
CREATE UNIQUE INDEX crm_external_identity_unique ON nodes(tenant_id,kind_id,(fields->>'external_provider'),(fields->>'external_id'))
 WHERE deleted_at IS NULL AND coalesce(fields->>'external_provider','')<>'' AND coalesce(fields->>'external_id','')<>'';
CREATE FUNCTION aeon_guard_crm_external_pair() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE slug text;
BEGIN
 SELECT k.slug INTO slug FROM node_kinds k WHERE k.tenant_id=NEW.tenant_id AND k.id=NEW.kind_id;
 IF slug IN ('organisation','contact') AND
   (coalesce(NEW.fields->>'external_provider','')='') IS DISTINCT FROM (coalesce(NEW.fields->>'external_id','')='') THEN
  RAISE EXCEPTION 'external provider and id must be set together';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER crm_external_pair_guard BEFORE INSERT OR UPDATE OF fields ON nodes FOR EACH ROW EXECUTE FUNCTION aeon_guard_crm_external_pair();
