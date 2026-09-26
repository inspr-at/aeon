# Demo seed

`aeon demo seed --tenant <slug>` fills an existing tenant with fictional data for product screenshots. It refuses to run unless `AEON_ENV` is exactly `dev`. An unset variable is not dev. Run it again on the same tenant and it does not add a second copy.

The tenant must already exist (`aeon tenant create`). The command does not create a tenant and it does not sign anyone in with a real identity provider. It binds three fictional people through the tenant bootstrap helper, using the issuer `https://demo.aeon.invalid`:

- Demo Operator, admin
- Ivo Quill, member
- Nia Frost, member

What the first successful run writes through the application modules and operator key store:

- Three projects: Lumen Archive (`LUMEN`), Harbor Ledger (`HARBOR`), and North Glass (`NGLASS`), each with a description.
- About 40 tickets under epics, with mixed states, assignees, comments, and relations. One Lumen ticket has a long fictional brief. Attachments are not seeded.
- A journey on Lumen Archive that reaches the Build stage, with an open release and tickets selected into it. Personal profile, an accepted brief, agreed requirements, and a person-approved build gate.
- Eight knowledge entries covering runbook, guideline, memory, external system, and related project, linked by `[[slug]]` mentions.
- Two agents, Lumen Scribe and Harbor Clerk. Scribe has a Codex harness session and one completed run on a work order. Clerk has one approval request left pending.
- A few hour entries on a fictional cost unit, priced in exact EUR decimals.

The seed commits all its changes in one database transaction. An interrupted run rolls back its nodes, keys, bindings, and events; a retry starts cleanly. The completion marker is `fields.demo_seed` = `complete` on the Lumen Archive project node. A later run reads that marker and returns without new events.

Journey gates use scopes `journey.requirements` and `journey.build`. Those names are what the journey module checks. They are not keys in the permission registry, so `aeon agent-key create` rejects them. The seed creates Scribe's key through the operator key store with registry scopes, then extends that exact key with the two journey prefixes and records `agent_key.scopes_extended` with its before and after scopes. Proposals and decisions after that go through the approvals API. The key token is kept in the process and is not printed.

Money in the seed is an exact decimal rate (`80.00` internal, `140.00` bill, EUR per hour). Durations are whole seconds. Nothing is stored as a binary float.

`demo.Seed(ctx, pool, slug)` is the same work without flag parsing, for tests. Both refuse a non-dev `AEON_ENV`. The seed is a CLI command only; it has no HTTP route.
