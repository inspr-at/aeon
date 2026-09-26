# Planning hierarchy

Aeon keeps one work tree. A project is not a separate table. It is a node whose kind slug is `project`, and it follows the same parent, relation, search, history, and move rules as every other node.

## Nodes and kinds

A node has a tenant, an immutable key, a kind, a title, a body, a JSON fields object, a state, a parent, and a decimal position among its siblings. Delete is soft. The key looks like `PRJ-4` or `LUMEN-1`: a short prefix, a hyphen, and a positive integer. The tenant allocates the next number in the same transaction as the insert. An explicit key is kept and advances that prefix's counter.

Kinds belong to the tenant. The slug is immutable because parent rules name it. The label, short prefix, icon, allowed child slugs, and JSON Schema for `fields` are configuration. A null child list permits any child. An empty list permits none. Narrowing the list affects new children only. Existing children stay movable and deletable.

New tenants start with project, epic, ticket, task, release, memory, runbook, and guideline. Later modules add kinds such as work_order, requirement, and the business kinds (cost_unit, organisation, contact, quote) when those features are configured. `external_system` and `related_project` are created on first use.

State is a non-empty string chosen by the tenant. The product screens treat `new` and `backlog` as open, `in_progress` as doing, `done` as finished, and `cancelled` as dropped. Knowledge uses its own trio: active is stored as `backlog`, proposed as `proposed`, and archived as `cancelled`.

## Relations

Relations are directed links between two live nodes: `blocks`, `relates`, `implements`, `cites`, and `duplicates`. `relates` is symmetric and stored with the smaller id as the source. CRM adds `customer_of` and `contact_for`, checked for kind and direction. A relation write needs permission on both ends. The change is one tenant event.

## Projects

`GET /api/projects` lists project nodes. The route key people use in `/p/:projectKey` is `fields.project_key` when set, otherwise the classic key, otherwise the prefix of the node key. The node key itself never changes. Moving a node into another project is a separate project-move, not an edit of the key.

## Releases

A release is a node of kind `release`. The journey projection in `journey_releases` records its sequence, state, and optional version. States used by the journey include `planning`, `building`, `candidate`, `deploying`, `refused`, `access`, `released`, and `superseded`. Creating a release and moving it between those states belongs to the journey actions, not to a free-form edit of the node. The release walker (`GET /api/projects/{id}/releases/{releaseId}/walker`) lists features and tickets selected into that release. A plan PUT stores the ordered ticket set. Ticket nodes can also exist in the project backlog without being in a release.

## Journey stages

Each project has one journey. The stage is derived. It is not a field a client can set. The eight stages, in order, are:

1. Inspire
2. Shape
3. Requirements
4. Plan
5. Build
6. Deploy
7. Access
8. Live

Derivation reads an accepted brief, the human Shape decision, the agreed requirements revision, the current release, human gate decisions, and terminal stage-handoff results. A heartbeat, a timer, or a stage string from the client does not move the rail. The personal profile skips Shape after a brief is confirmed. Professional and enterprise profiles add budget and, for enterprise, a separate reviewer. Access is skipped only when the release has no explicit access change.

Human gates are approval requests on the project or release node (`journey.shape`, the revision-bound requirements scope, `journey.build`, `journey.candidate`, `journey.deploy`, `journey.access`). An agent proposes. A person decides. The journey action checks that live approval and then writes its own event. Functional requirements are requirement nodes. Agreement creates one epic per requirement and can generate tickets from accepted suggestions. A manual ticket that changes agreed scope stays marked until requirements are agreed again.

`GET /api/projects/{projectId}/journey` returns the stage, the single next action, and whether that action is available. The first person action initializes a missing projection.

## Knowledge

Knowledge entries are ordinary nodes: runbook, guideline, memory, external system, and related project. The CLI type uses a hyphen (`external-system`). The kind slug uses an underscore (`external_system`). `fields.slug` is the stable name, matching `^[a-z][a-z0-9_-]*$`, at most 64 characters, unique among live entries of that type in the project.

`GET /api/knowledge` lists them. `GET /api/knowledge/graph` builds a graph from directed relations and from mentions in bodies: wiki links of the form `[[slug]]`, Markdown links, and code slugs. Same-kind slugs in the project win over other kinds. The graph is a read. It does not write an event. Creating, editing, or deleting an entry does, as `knowledge.created`, `knowledge.updated`, or `knowledge.deleted`.
