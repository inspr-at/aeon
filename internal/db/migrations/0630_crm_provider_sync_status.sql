-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE crm_provider_sync_status (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  organisation_node_id uuid NOT NULL,
  provider_id text NOT NULL,
  state text NOT NULL CHECK (state IN ('ok','error')),
  attempted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  synced_at timestamptz,
  last_error text NOT NULL DEFAULT '' CHECK (length(last_error) <= 120),
  PRIMARY KEY (tenant_id,organisation_node_id),
  FOREIGN KEY (tenant_id,organisation_node_id) REFERENCES nodes(tenant_id,id),
  CHECK ((state='ok' AND synced_at IS NOT NULL AND last_error='') OR state='error')
);
ALTER TABLE crm_provider_sync_status ENABLE ROW LEVEL SECURITY;
ALTER TABLE crm_provider_sync_status FORCE ROW LEVEL SECURITY;
CREATE POLICY crm_provider_sync_status_tenant ON crm_provider_sync_status
  USING (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid)
  WITH CHECK (tenant_id=NULLIF(current_setting('aeon.tenant_id',true),'')::uuid);
