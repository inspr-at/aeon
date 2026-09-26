#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# AEON-132: local-only Phase B proof. Never points at classic or production.
set -Eeuo pipefail

root=$(git rev-parse --show-toplevel)
dump=${AEON_CR1_DUMP:-/Users/markus/Code/aeon-worktrees/cutover/aeon-prod-copy.dump}
fixture="$root/scripts/cutover/classic-fixture.json"
[[ -f "$dump" ]] || { printf 'CR1 dump absent: %s\n' "$dump" >&2; exit 1; }
[[ -f "$fixture" ]] || { printf 'CR1 recorded fixture absent\n' >&2; exit 1; }
for tool in docker python3 jq curl go trash rg; do
    command -v "$tool" >/dev/null || { printf 'missing %s\n' "$tool" >&2; exit 1; }
done

tmp=$(mktemp -d "${TMPDIR:-/tmp}/aeon-cr1.XXXXXX")
container="aeon-cr1-$$"
fixture_pid=''
server_pid=''
cleanup() {
    if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
    if [[ -n "$fixture_pid" ]]; then kill "$fixture_pid" 2>/dev/null || true; wait "$fixture_pid" 2>/dev/null || true; fi
    docker stop "$container" >/dev/null 2>&1 || true
    trash "$tmp"
}
trap cleanup EXIT

# An ephemeral container gets its own loopback-only port and disappears on stop.
docker run -d --rm --name "$container" -p 127.0.0.1::5432 \
    -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=aeon_cr1 \
    pgvector/pgvector:pg18 >/dev/null
for _ in {1..60}; do
    if docker exec "$container" pg_isready -U postgres -d aeon_cr1 >/dev/null 2>&1; then break; fi
    sleep 1
done
docker exec "$container" pg_isready -U postgres -d aeon_cr1 >/dev/null
docker exec "$container" psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
    -c "CREATE ROLE aeon LOGIN PASSWORD 'aeon'" >/dev/null
db_port=$(docker port "$container" 5432/tcp)
db_port=${db_port##*:}
[[ "$db_port" =~ ^[0-9]+$ ]] || { printf 'invalid local database port\n' >&2; exit 1; }
export AEON_DATABASE_URL="postgres://aeon:aeon@127.0.0.1:${db_port}/aeon_cr1?sslmode=disable"
export AEON_ENV=dev AEON_BOOTSTRAP_TENANT_SLUG=cr1rehearsal
export AEON_BOOTSTRAP_TENANT_NAME='CR1 Rehearsal'
export AEON_FILES_DIR="$tmp/files"

restore() {
    docker exec -i "$container" pg_restore --no-owner --no-acl --exit-on-error \
        -U postgres -d aeon_cr1 < "$dump"
    docker exec "$container" psql -U postgres -d aeon_cr1 -v ON_ERROR_STOP=1 \
        -c 'GRANT USAGE, CREATE ON SCHEMA public TO aeon; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO aeon; GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO aeon; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO aeon' >/dev/null
}
fingerprint() {
    docker exec "$container" psql -U postgres -d aeon_cr1 -Atc \
        "SELECT (SELECT count(*) FROM nodes)::text || ':' || \
                (SELECT md5(string_agg(id::text || key || md5(fields::text), ',' ORDER BY id)) FROM nodes) || ':' || \
                (SELECT count(*) FROM events)::text || ':' || \
                (SELECT md5(string_agg(tenant_id::text || id::text || type || md5(coalesce(after::text,'')), ',' ORDER BY tenant_id,id)) FROM events) || ':' || \
                (SELECT count(*) FROM tenants)::text"
}
restore
before=$(fingerprint)
printf 'CR1: restored supplied dump into disposable local Postgres\n'

printf 'cr1-recorded-fixture\n' > "$tmp/fixture-key"
chmod 600 "$tmp/fixture-key"
python3 "$root/scripts/cutover/fixture-server.py" \
    --fixture "$fixture" --port-file "$tmp/fixture-port" > "$tmp/fixture.log" 2>&1 &
fixture_pid=$!
for _ in {1..40}; do [[ -s "$tmp/fixture-port" ]] && break; sleep 0.1; done
[[ -s "$tmp/fixture-port" ]] || { printf 'recorded fixture did not start\n' >&2; exit 1; }
fixture_port=$(<"$tmp/fixture-port")
[[ "$fixture_port" =~ ^[0-9]+$ ]] || exit 1
source_url="http://127.0.0.1:${fixture_port}"
curl --silent --fail --header 'Authorization: Bearer cr1-recorded-fixture' \
    "$source_url/api/projects?status=all" >/dev/null

go build -o "$tmp/aeon" ./cmd/aeon
ln -s "$tmp/aeon" "$tmp/paimos"
"$tmp/aeon" tenant create --slug cr1rehearsal --name 'CR1 Rehearsal' > "$tmp/tenant.json"
import_args=(--source-url "$source_url" --api-key-file "$tmp/fixture-key" \
    --tenant cr1rehearsal --project CR1FIX)
"$tmp/aeon" import reconcile "${import_args[@]}" > "$tmp/reconcile-before.json"
first_delta=$(jq '[.projects[].categories[] | (.missing|length)+(.extra|length)+(.changed|length)] | add // 0' "$tmp/reconcile-before.json")
[[ "$first_delta" -gt 0 ]] || { printf 'fixture did not produce a real pre-import delta\n' >&2; exit 1; }
"$tmp/aeon" import paimos "${import_args[@]}" --delta > "$tmp/import.json"
jq -e '.created > 0 and (.conflicts|length) == 0 and (.skipped|length) == 0' "$tmp/import.json" >/dev/null
"$tmp/aeon" import reconcile "${import_args[@]}" > "$tmp/reconcile-after.json"
jq -e '.partial == false and .skipped_count == 0 and
    ([.projects[].categories[] | (.missing|length)+(.extra|length)+(.changed|length)] | add // 0) == 0' \
    "$tmp/reconcile-after.json" >/dev/null
printf 'CR1: reconcile delta %s -> 0 after aeon import paimos --delta\n' "$first_delta"

# CLI uses the same argv[0]=paimos path as the eventual compatibility binary.
# Keys exist only in this disposable database and private temporary directory.
"$tmp/aeon" agent-key create --tenant cr1rehearsal --name cr1worker \
    --scopes nodes.read,nodes.write,comments.write,events.read,knowledge.read,knowledge.write,models.read,inbox.send \
    --workspace-role member --out-file "$tmp/worker-key" > "$tmp/worker.json"
"$tmp/aeon" agent-key create --tenant cr1rehearsal --name cr1peer \
    --scopes nodes.read,inbox.read,inbox.send --workspace-role member \
    --out-file "$tmp/peer-key" > "$tmp/peer.json"
peer_id=$(jq -r '.principal_id' "$tmp/peer.json")
[[ "$peer_id" =~ ^[0-9a-f-]{36}$ ]] || exit 1
peer_address=codex:cr1peer
app_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
export AEON_ADDR="127.0.0.1:${app_port}" AEON_PUBLIC_URL="http://127.0.0.1:${app_port}"
export PAIMOS_URL="$AEON_PUBLIC_URL" PAIMOS_API_KEY_FILE="$tmp/worker-key"
"$tmp/aeon" serve > "$tmp/server.log" 2>&1 &
server_pid=$!
for _ in {1..60}; do
    if curl --silent --fail "$AEON_PUBLIC_URL/api/health" >/dev/null 2>&1; then break; fi
    kill -0 "$server_pid" 2>/dev/null || { printf 'rehearsal server failed to start\n' >&2; exit 1; }
    sleep 1
done
curl --silent --fail "$AEON_PUBLIC_URL/api/health" >/dev/null

printf 'CR1: CLI issue create\n'
"$tmp/paimos" --json issue create --project CR1FIX --title 'CR1 local smoke ticket' \
    --description 'Disposable rehearsal only' --type ticket > "$tmp/created.json"
issue_key=$(jq -r '.issue_key' "$tmp/created.json")
[[ "$issue_key" == CR1FIX-* ]] || exit 1
"$tmp/paimos" issue comment "$issue_key" --body 'CR1 smoke comment' > "$tmp/comment.out"
"$tmp/paimos" issue update "$issue_key" --status in-progress > "$tmp/update.out"
"$tmp/paimos" --json session start --project CR1FIX --agent cr1worker > "$tmp/session.json"
session_id=$(jq -r '.session_id' "$tmp/session.json")
[[ "$session_id" =~ ^[0-9a-f-]{36}$ ]] || exit 1
PAIMOS_AGENT_NAME=cr1worker PAIMOS_SESSION_ID="$session_id" \
    "$tmp/paimos" issue comment "$issue_key" --body 'CR1 session marker comment' > "$tmp/marker.out"
"$tmp/paimos" --json issue get "$issue_key" > "$tmp/issue.json"
jq -e '.status == "in-progress" and ([.comments[] | select(. == "CR1 session marker comment")] | length) == 1' \
    "$tmp/issue.json" >/dev/null
"$tmp/paimos" --json model resolve build > "$tmp/model.json"
jq -e '.profile != null and .owner_required == false' "$tmp/model.json" >/dev/null
printf 'CR1: CLI knowledge create/list/get\n'
"$tmp/paimos" knowledge create --type memory --slug cr1-note --project CR1FIX \
    --title 'CR1 CLI note' --body 'Disposable knowledge' > "$tmp/knowledge-create.out"
"$tmp/paimos" knowledge list --project CR1FIX > "$tmp/knowledge-list.out"
"$tmp/paimos" knowledge get memory cr1-note --project CR1FIX > "$tmp/knowledge-get.out"
rg -q 'cr1-note' "$tmp/knowledge-list.out" "$tmp/knowledge-get.out"
"$tmp/paimos" --json tell "$peer_address" --project CR1FIX -m 'CR1 local inbox smoke' > "$tmp/tell.json"
jq -e --arg address "$peer_address" '.to == $address and .message_id != null' "$tmp/tell.json" >/dev/null
PAIMOS_API_KEY_FILE="$tmp/peer-key" "$tmp/paimos" listen --as "$peer_address" --project CR1FIX \
    --ack > "$tmp/listen.out"
rg -q 'CR1 local inbox smoke' "$tmp/listen.out"
printf 'CR1: paimos compat issue, session marker, model, knowledge, tell/listen passed\n'

after=$(fingerprint)
[[ "$after" != "$before" ]] || { printf 'rehearsal made no database change\n' >&2; exit 1; }
kill "$server_pid" 2>/dev/null || true
wait "$server_pid" 2>/dev/null || true
server_pid=''
docker exec "$container" psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
    -c 'DROP DATABASE aeon_cr1 WITH (FORCE)' >/dev/null
docker exec "$container" psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
    -c 'CREATE DATABASE aeon_cr1' >/dev/null
restore
rolled_back=$(fingerprint)
[[ "$rolled_back" == "$before" ]] || { printf 'rollback fingerprint differs from original restore\n' >&2; exit 1; }
remaining=$(docker exec "$container" psql -U postgres -d aeon_cr1 -Atc \
    "SELECT count(*) FROM nodes WHERE key='CR1FIX-1' OR title='CR1 local smoke ticket'")
[[ "$remaining" == 0 ]] || { printf 'rehearsal records survived rollback\n' >&2; exit 1; }
printf 'CR1: drop/restore rollback matches baseline node, event and tenant fingerprints\n'
