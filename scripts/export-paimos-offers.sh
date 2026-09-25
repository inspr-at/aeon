#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Usage: scripts/export-paimos-offers.sh INSTANCE CUSTOMER_ID OUTPUT_DIR
# GET-only EQ0 bundle. Requires paimos CLI and jq. Never packages credentials.
# To stream later: tar -C OUTPUT_DIR -cf - . | aeon import paimos-offers \
#   --source-instance INSTANCE --bundle - [--tenant-id UUID --actor-principal-id UUID --apply]
set -euo pipefail

if [[ $# != 3 || ! $2 =~ ^[1-9][0-9]*$ ]]; then
  echo 'usage: export-paimos-offers.sh INSTANCE CUSTOMER_ID OUTPUT_DIR' >&2
  exit 2
fi
instance=$1
customer_id=$2
out=$3
mkdir -p "$out"
paimos --instance "$instance" curl --method GET "/api/customers/$customer_id" > "$out/customer-$customer_id.json"
paimos --instance "$instance" curl --method GET "/api/customers/$customer_id/contacts" > "$out/customer-$customer_id-contacts.json"
paimos --instance "$instance" curl --method GET "/api/customers/$customer_id/offers" > "$out/customer-$customer_id-offers.json"
paimos --instance "$instance" curl --method GET '/api/integrations/crm/offers' > "$out/offer-settings.json"
paimos --instance "$instance" curl --method GET '/api/branding' > "$out/branding.json"
while IFS= read -r offer_id; do
  paimos --instance "$instance" curl --method GET "/api/offers/$offer_id" > "$out/offer-$offer_id.json"
  jq '.document' "$out/offer-$offer_id.json" > "$out/offer-$offer_id-document.json"
done < <(jq -r '.[].id' "$out/customer-$customer_id-offers.json")
