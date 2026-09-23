-- SPDX-License-Identifier: AGPL-3.0-only

-- Sessions are global: the cookie arrives before a tenant is known, so this
-- table has no row-level security. The id is the hex sha256 of the cookie token.
CREATE TABLE sessions (
    id text PRIMARY KEY,
    identity_id uuid NOT NULL REFERENCES identities (id),
    tenant_id uuid NOT NULL REFERENCES tenants (id),
    principal_id uuid NOT NULL REFERENCES principals (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_expires_at ON sessions (expires_at);
