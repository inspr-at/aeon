-- SPDX-License-Identifier: AGPL-3.0-only
-- QL1/AEON-109: customers can be archived and restored. An archived customer
-- leaves the default Customers list and keeps its contacts, numbers, projects
-- and quotes; archiving never deletes and is undone through its event.
ALTER TABLE crm_organisation_profiles ADD COLUMN archived_at timestamptz;
ALTER TABLE crm_organisation_profiles ADD COLUMN archived_by_principal_id uuid;
ALTER TABLE crm_organisation_profiles ADD CONSTRAINT crm_organisation_archive_actor_fk
  FOREIGN KEY (tenant_id, archived_by_principal_id) REFERENCES principals(tenant_id, id);
ALTER TABLE crm_organisation_profiles ADD CONSTRAINT crm_organisation_archive_actor_pair
  CHECK ((archived_at IS NULL) = (archived_by_principal_id IS NULL));
