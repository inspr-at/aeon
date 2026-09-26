# Agent integration

Aeon is the interface for people and for agents. People sign in with OIDC. Agents call the HTTP API with a scoped key, or they talk to a local `aeon-agentd` which holds that key and supervises a harness on the operator's machine.

## CLI and paimos mode

`aeon` is one binary. `aeon serve` runs the server; operator commands such as `aeon demo seed` and the agent CLI are dispatched by the same binary. The same binary behaves as `paimos` when `argv[0]` is `paimos` (the Nix package installs that name as a symlink). Help text uses the name you invoked.

Configuration lives in `~/.aeon/config.yaml`, or `~/.paimos/config.yaml` when the binary is `paimos`. The file names a default instance and a map of instances, each with a URL. API keys are not kept in that file. They are stored under the sibling `keys/` directory, mode `0600`. If a key is still in the YAML, the CLI moves it there on the next read.

A process can skip the file. `AEON_URL` together with `AEON_API_KEY` or `AEON_API_KEY_FILE` is the target. When the binary is `paimos` and `AEON_URL` is unset, `PAIMOS_URL` with `PAIMOS_API_KEY` or `PAIMOS_API_KEY_FILE` works the same way. If `AEON_URL` is set, it wins even for the `paimos` name. Do not set both the key variable and the key file.

`aeon auth login` records an instance. The key is read from `--key-file` or stdin, never echoed. `aeon whoami` calls `GET /api/me`.

Issue, project, knowledge, search, relation, tag, attachment, model resolve, and onboard commands call the configured Aeon server. `issue list` reads issue nodes beneath a project. Knowledge types on the CLI are `memory`, `runbook`, `guideline`, `external-system`, and `related-project`.

## aeon-agentd

`aeon-agentd` is a separate binary (`cmd/aeon-agentd`). It is the operator-local harness supervisor. It is not an `aeon` subcommand.

`aeon-agentd serve` takes `--url`, `--agent-key-file` (an absolute path, mode `0600`), a workspace, a private state directory, a stable `--daemon-id`, and local account settings. At least one positive per-run reservation flag is required: `--estimate-requests`, `--estimate-tokens`, or `--estimate-cost-micros`. It authenticates to Aeon with the scoped key. The URL must use HTTPS except that HTTP is accepted on `localhost`, `127.0.0.1`, or `::1`. It pulls work and inbox items, and it reports content-free telemetry and receipts. It does not send vendor tokens or raw vendor protocol payloads. A webhook from Aeon is only a hint to fetch state. A heartbeat is not proof that this daemon still owns a process.

`aeon-agentd control` sends a fenced local control. Vendor session ids stay on the daemon. Aeon stores the run id and the agent principal.

The daemon adapts Codex, Claude, Pi, Cursor, and Grok locally. The native Grok adapter requires macOS. Aeon sees a harness name, an opaque account key, a family, and bounded usage counters.

## Agent keys and scopes

An operator creates a key with `aeon agent-key create --tenant SLUG (--name AGENT | --principal-id UUID) --out-file PATH`. The token is written only to that file (mode `0600`, never overwritten) and is not printed. `aeon agent-key revoke` revokes by id.

Use `--workspace-role ROLEKEY` when the agent needs workspace access. This binds the agent principal as part of key creation; its effective permissions are the intersection of the key scopes and that role. The operator cannot grant Owner or workspace Guest. For an existing principal, use `aeon access bind --tenant SLUG --principal NAME_OR_UUID --workspace-role ROLEKEY`; remove the binding with `aeon access unbind --tenant SLUG --principal NAME_OR_UUID --workspace-role`. Repeating either action is safe. These commands use the same workspace binding rules and audit event as the Members API.

Scopes are an outer ceiling. An empty list grants nothing. Unknown names and permissions that are not agent-grantable are rejected. Typical scopes are registry keys such as `nodes.read`, `nodes.write`, `inbox.send`, `intake.write`, `approvals.request`, `work_orders.write`, `run.create`, `run.claim`, `run.telemetry`, `harness.write`, and `account.manage`. For HTTP key creation, the creating principal must hold every requested scope. The operator-only `aeon agent-key create` command runs on the host without a creating principal; it can create an agent principal and does not perform that creator-permission check. It still validates requested scopes against the registry.

The HTTP middleware applies that ceiling before module handlers run. Routes with no agent mapping answer 403. Person sessions are governed by role instead. An approval grant cannot exceed the key. Journey gate scopes such as `journey.build` are checked against this ceiling. They are not themselves keys in the permission registry, so `aeon agent-key create` will not accept them. A key that already holds a registry prefix covers dotted refinements of that prefix (`nodes.read` covers `nodes.read.fields`).

## Operator project access

On the AEON host, with `AEON_DATABASE_URL` set for the tenant database, an operator can create an agent key and grant project access together:

```sh
aeon agent-key create --tenant inspr --name pharos-worker --out-file /secure/path/pharos.key --scopes nodes:read --project PHAROS_PROJECT_KEY --project-role viewer
aeon agent-key create --tenant inspr --name janus-worker --out-file /secure/path/janus.key --scopes nodes:read --project JANUS_PROJECT_KEY=viewer
```

Repeat `--project KEY --project-role ROLEKEY` or use a comma-separated `KEY=ROLE` list for several projects. The key goes only to the specified new 0600 file. If binding fails after key creation, the command reports the key ID and file path so the operator can correct access or revoke the key.

For existing people or agents, use `aeon access bind --tenant SLUG --principal NAME_OR_UUID --project KEY --role ROLEKEY` and `aeon access unbind --tenant SLUG --principal NAME_OR_UUID --project KEY`. An ambiguous name returns candidate UUIDs. Owner and Customer are workspace roles and cannot be granted on a project. Repeating a bind to the same role or an unbind adds no event. These commands run locally with database access; they are not HTTP endpoints. Project mutations use the same store path as the members API and append `binding.set` or `binding.removed` events. Operator CLI events use the per-tenant Access operator principal as actor.

## Inbox

Messages are durable rows in one tenant. Each names a sender, a recipient, an idempotency key, and one sent event. The body is not copied into the tenant-wide event payload or into a webhook.

`aeon tell` sends. The target is a `harness:agent` address or a principal UUID, with `--project` and the message text. `aeon listen --project KEY` reads the caller's inbox (at most 10 rows). `--ack` acknowledges each message that was printed. Only the recipient can acknowledge, and only an acknowledgement removes the message from the pending view. Repeating an acknowledgement is safe. `aeon message deliveries --project KEY` shows redacted delivery state, not message bodies.

Long poll holds at most 30 seconds. A webhook or a notify is a hint. The daemon then fetches authenticated state. The webhook body is ids only and is not authority.

## Harness sessions

A harness session is a public generation of a local worker on one project. `POST /api/projects/{projectId}/harness-sessions` registers it. The body names the agent principal, the harness (`codex`, `claude`, `pi`, `cursor`, or `grok`), the host, managed or unmanaged mode, worker or coordinator role, advertised capabilities, a session ref, and a worker lease. A ticket and a work shape (`ship` or `scout`) are bound together. A run, when set, must belong to that agent and to the named work order.

The same session ref and lease register again without a second row. A different active registration for that ref conflicts. Worker calls (heartbeat, yield, drain, stop) require the agent, the lease, and a harness worker scope. People can interrupt or stop a session with a harness control scope.

`GET /api/harness-sessions` lists sessions across projects, with state, harness, agent, project, and ticket filters.

## Work orders and runs

A work order is a `work_order` node plus assignment, status, acceptance criteria, and optional cost and time ceilings. Cost is integer micros. Duration is whole seconds. Status moves through `draft`, `ready`, `running`, `blocked`, `done`, and `cancelled`. `done` requires every criterion checked, evidence attached, and no live run.

`POST /api/work-orders/{id}/runs` queues a run for an agent and a model profile. The order must be `ready` or `running`, and under its ceilings. The caller is the assigned agent or holds `run.create` as a person. Routing picks an agent account with a fresh probe and allowance, and reserves the estimate. Claim binds the daemon id and generation. Telemetry then reports monotonic counters and a status. A finished report uses a terminal status (`completed`, `failed`, `cancelled`, or `ownership_lost`) and sets `ended_at`. Exact replay of the same sequence is idempotent. A divergent replay conflicts. Vendor text and secrets are not stored in telemetry.

Model profiles are tenant pins (harness, family, model, effort, tier). The first read of an empty registry seeds the catalog and records that with an event. `aeon model resolve` asks the server which profile a role should use. It does not execute a command.

## Stage handoff launch

The coordinator mounts `stagehandoff.New` (an `httpapi.Module`) with a Pharos launch-check provider and registers the compiled Pharos manifest through `plugins.Builtin`. A routed Pharos agent sends a UUID `Idempotency-Key` on each admit and consume request. Retry the same key and JSON body after a lost response: Aeon returns the original admission or consumption receipt, including `consumed_at`, without repeating the change. A changed body or principal conflicts; a new key cannot consume an already spent admission. Exact replay remains available for 24 hours after a terminal handoff. `GET /api/stage-handoffs/{id}` includes safe admission state for reconciliation.

## MCP

`aeon mcp` (or `paimos mcp`) serves a stdio MCP server. `whoami` calls `GET /api/me` on the configured instance. The server also registers `issue_list`, `issue_get`, `issue_create`, `issue_update`, `issue_comment`, `knowledge_list`, `knowledge_get`, `knowledge_create`, `knowledge_update`, and `search`. Those tools still answer that they arrive in R1. Use the CLI verbs for those operations. They already call the Aeon API.
