// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
)

var (
	errNotMember = errors.New("not a member")
	errNotFound  = errors.New("not found")
)

func scanPrincipal(row pgx.Row) (tenant.Principal, error) {
	var p tenant.Principal
	var kind string
	var roles pgtype.FlatArray[string]
	if err := row.Scan(&p.ID, &p.TenantID, &kind, &p.Name, &roles); err != nil {
		return tenant.Principal{}, err
	}
	p.Kind = tenant.PrincipalKind(kind)
	p.Roles = []string(roles)
	if p.Roles == nil {
		p.Roles = []string{}
	}
	return p, nil
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (m *Module) tenantBySlug(ctx context.Context, slug string) (string, error) {
	return tenantbootstrap.ResolveSlug(ctx, m.pool, slug)
}

// resolveOIDCPerson resolves issuer+subject only within the signed target
// tenant. A bootstrap email may create the original tenant admin, but cannot
// enroll itself in any additional tenant.
func (m *Module) resolveOIDCPerson(ctx context.Context, tenantID, slug, issuer, subject, email, name string) (tenant.Principal, string, error) {
	var p tenant.Principal
	var identityID string
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		var emailArg, displayArg any
		if strings.TrimSpace(email) != "" {
			emailArg = strings.TrimSpace(email)
		}
		if strings.TrimSpace(name) != "" {
			displayArg = strings.TrimSpace(name)
		}
		var oldEmail, oldDisplay *string
		var hadIdentity bool
		err := tx.QueryRow(ctx, `SELECT email,display_name FROM identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&oldEmail, &oldDisplay)
		if err == nil {
			hadIdentity = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO identities(issuer,subject,email,display_name)
			VALUES($1,$2,$3,$4) ON CONFLICT(issuer,subject) DO UPDATE
			SET email=COALESCE(EXCLUDED.email,identities.email),
			    display_name=COALESCE(EXCLUDED.display_name,identities.display_name)
			RETURNING id::text`, issuer, subject, emailArg, displayArg).Scan(&identityID); err != nil {
			return err
		}
		p, err = scanPrincipal(tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,kind,name,roles
			FROM principals WHERE tenant_id=$1::uuid AND identity_id=$2::uuid AND kind='person'`, tenantID, identityID))
		if err == nil {
			var newEmail, newDisplay *string
			if err := tx.QueryRow(ctx, `SELECT email,display_name FROM identities WHERE id=$1::uuid`, identityID).Scan(&newEmail, &newDisplay); err != nil {
				return err
			}
			if hadIdentity && (!sameNullable(oldEmail, newEmail) || !sameNullable(oldDisplay, newDisplay)) {
				_, err = events.Append(ctx, tx, p, events.Change{Type: "identity.updated",
					Before: map[string]any{"identity_id": identityID, "email": oldEmail, "display_name": oldDisplay},
					After:  map[string]any{"identity_id": identityID, "email": newEmail, "display_name": newDisplay}})
				return err
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if slug != m.cfg.BootstrapTenantSlug || !adminEmail(email, m.cfg.BootstrapAdminEmail) {
			return errNotMember
		}
		p, err = scanPrincipal(tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,identity_id,name,roles)
			VALUES($1::uuid,'person',$2::uuid,$3,ARRAY['admin'])
			ON CONFLICT(tenant_id,identity_id) WHERE identity_id IS NOT NULL
			DO NOTHING
			RETURNING id::text,tenant_id::text,kind,name,roles`, tenantID, identityID, name))
		if errors.Is(err, pgx.ErrNoRows) {
			p, err = scanPrincipal(tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,kind,name,roles
				FROM principals WHERE tenant_id=$1::uuid AND identity_id=$2::uuid AND kind='person'`, tenantID, identityID))
			return err
		}
		if err != nil {
			return err
		}
		_, err = events.Append(ctx, tx, p, events.Change{Type: "tenant.principal_bound",
			After: map[string]any{"principal_id": p.ID, "issuer": issuer, "subject": subject, "name": p.Name, "roles": p.Roles}})
		return err
	})
	return p, identityID, err
}

func sameNullable(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (m *Module) upsertIdentity(ctx context.Context, tenantID, issuer, subject, email, display string) (string, error) {
	var emailArg, displayArg any
	if strings.TrimSpace(email) != "" {
		emailArg = strings.TrimSpace(email)
	}
	if strings.TrimSpace(display) != "" {
		displayArg = strings.TrimSpace(display)
	}
	var id string
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO identities (issuer, subject, email, display_name)
			VALUES ($1, $2, $3, $4) ON CONFLICT (issuer, subject) DO NOTHING`, issuer, subject, emailArg, displayArg); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id::text FROM identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&id)
	})
	return id, err
}

// ensurePerson returns the person principal for this identity inside the tenant.
// The bootstrap admin email is provisioned with the admin role on first sign-in.
func (m *Module) ensurePerson(ctx context.Context, tenantID, identityID, email, name string) (tenant.Principal, error) {
	var p tenant.Principal
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		p, err = scanPrincipal(tx.QueryRow(ctx, `
			SELECT id::text, tenant_id::text, kind, name, roles
			FROM principals
			WHERE identity_id = $1::uuid AND kind = 'person'
		`, identityID))
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !adminEmail(email, m.cfg.BootstrapAdminEmail) {
			return errNotMember
		}
		if strings.TrimSpace(name) == "" {
			name = strings.TrimSpace(email)
		}
		if _, err := tx.Exec(ctx, "SAVEPOINT person_insert"); err != nil {
			return err
		}
		p, err = scanPrincipal(tx.QueryRow(ctx, `
			INSERT INTO principals (tenant_id, kind, identity_id, name, roles)
			VALUES ($1::uuid, 'person', $2::uuid, $3, $4)
			RETURNING id::text, tenant_id::text, kind, name, roles
		`, tenantID, identityID, name, []string{"admin"}))
		if isUnique(err) {
			if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT person_insert"); rbErr != nil {
				return rbErr
			}
			p, err = scanPrincipal(tx.QueryRow(ctx, `
				SELECT id::text, tenant_id::text, kind, name, roles
				FROM principals
				WHERE identity_id = $1::uuid AND kind = 'person'
			`, identityID))
			return err
		}
		if err != nil {
			return err
		}
		if _, err := events.Append(ctx, tx, p, events.Change{Type: "tenant.principal_bound",
			After: map[string]any{"principal_id": p.ID, "name": p.Name, "roles": p.Roles}}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "RELEASE SAVEPOINT person_insert")
		return err
	})
	return p, err
}

func adminEmail(got, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(got), want)
}

// personByEmail resolves dev-login email to its canonical person in the tenant.
// Prefer link targets over older imported identities with the same email.
func (m *Module) personByEmail(ctx context.Context, tenantID, email string) (tenant.Principal, string, error) {
	var p tenant.Principal
	var identityID string
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		var kind string
		var roles pgtype.FlatArray[string]
		err := tx.QueryRow(ctx, `
			SELECT canonical.id::text, canonical.tenant_id::text, canonical.kind,
			       canonical.name, canonical.roles, COALESCE(canonical.identity_id, p.identity_id)::text
			FROM principals p
			LEFT JOIN identities i ON i.id = p.identity_id
			JOIN principals canonical ON canonical.tenant_id=p.tenant_id
			    AND canonical.id=COALESCE(p.linked_to,p.id)
			WHERE p.tenant_id=$2::uuid AND p.kind='person'
			    AND lower(COALESCE(NULLIF(p.email,''),i.email))=lower($1)
			    AND COALESCE(canonical.identity_id,p.identity_id) IS NOT NULL
			ORDER BY (EXISTS (SELECT 1 FROM principals source
			    WHERE source.tenant_id=p.tenant_id AND source.linked_to=canonical.id)) DESC,
			    (p.linked_to IS NULL) DESC, canonical.created_at, canonical.id, p.id
			LIMIT 1
		`, email, tenantID).Scan(&p.ID, &p.TenantID, &kind, &p.Name, &roles, &identityID)
		if err != nil {
			return err
		}
		p.Kind = tenant.PrincipalKind(kind)
		p.Roles = []string(roles)
		if p.Roles == nil {
			p.Roles = []string{}
		}
		return nil
	})
	return p, identityID, err
}

func (m *Module) startSession(ctx context.Context, identityID, tenantID, principalID string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO sessions (id, identity_id, tenant_id, principal_id, expires_at)
			VALUES ($1, $2::uuid, $3::uuid, $4::uuid, now() + interval '30 days')`, sessionID(raw), identityID, tenantID, principalID)
		return err
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func decodeSessionToken(value string) ([]byte, error) {
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return nil, errBadCookie
	}
	return raw, nil
}

func (m *Module) authenticateSession(ctx context.Context, raw []byte) (tenant.Principal, bool, error) {
	id := sessionID(raw)
	var tenantID, principalID string
	err := m.inTenant(ctx, m.pool, tenantbootstrap.LookupTenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('aeon.session_id',$1,true)`, id); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `UPDATE sessions
			SET last_seen_at = now(), expires_at = now() + interval '30 days'
			WHERE id = $1 AND expires_at > now()
			RETURNING tenant_id::text, principal_id::text`, id).Scan(&tenantID, &principalID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		_ = m.deleteSession(ctx, raw)
		return tenant.Principal{}, false, nil
	}
	if err != nil {
		return tenant.Principal{}, false, err
	}
	var p tenant.Principal
	err = m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		var scanErr error
		p, scanErr = displayPrincipal(ctx, tx, tenantID, principalID)
		return scanErr
	})
	if errors.Is(err, pgx.ErrNoRows) {
		_ = m.deleteSession(ctx, raw)
		return tenant.Principal{}, false, nil
	}
	if err != nil {
		return tenant.Principal{}, false, err
	}
	return p, true, nil
}

func (m *Module) deleteSession(ctx context.Context, raw []byte) error {
	return m.inTenant(ctx, m.pool, tenantbootstrap.LookupTenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('aeon.session_id',$1,true)`, sessionID(raw)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID(raw))
		return err
	})
}

// prefixFor builds aeon_<prefix>_<secret>'s prefix: 32 hex chars of the tenant
// uuid, then 16 hex chars of randomness. The tenant half lets the bearer
// resolver enter InTenant before it can see the row.
func prefixFor(tenantID string) (string, error) {
	compact := strings.ToLower(strings.ReplaceAll(tenantID, "-", ""))
	if len(compact) != 32 || !isHex(compact) {
		return "", fmt.Errorf("tenant id %q is not a uuid", tenantID)
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return compact + hex.EncodeToString(buf), nil
}

func tenantFromPrefix(prefix string) (string, bool) {
	if len(prefix) < 48 || !isHex(prefix[:32]) {
		return "", false
	}
	h := strings.ToLower(prefix[:32])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], true
}

func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return s != ""
}

func newSecret() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	secret := hex.EncodeToString(buf)
	return secret, hashSecret(secret), nil
}

func (m *Module) authenticateAgent(ctx context.Context, prefix, secret string) (tenant.Principal, bool, error) {
	tenantID, ok := tenantFromPrefix(prefix)
	if !ok {
		return tenant.Principal{}, false, nil
	}
	var p tenant.Principal
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		var principalID, gotTenant string
		var scopes pgtype.FlatArray[string]
		err := tx.QueryRow(ctx, `
			UPDATE agent_keys k
			SET last_used_at = now()
			WHERE k.prefix = $1 AND k.hash = $2
			  AND k.revoked_at IS NULL
			  AND (k.expires_at IS NULL OR k.expires_at > now())
			  AND EXISTS (
			    SELECT 1 FROM principals p
			    WHERE p.id = k.principal_id AND p.kind = 'agent'
			  )
			RETURNING k.principal_id::text, k.tenant_id::text, k.scopes
		`, prefix, hashSecret(secret)).Scan(&principalID, &gotTenant, &scopes)
		if err != nil {
			return err
		}
		if !strings.EqualFold(gotTenant, tenantID) {
			return pgx.ErrNoRows
		}
		p, err = scanPrincipal(tx.QueryRow(ctx, `
			SELECT id::text, tenant_id::text, kind, name, roles
			FROM principals WHERE id = $1::uuid
		`, principalID))
		p.Scopes = []string(scopes)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return tenant.Principal{}, false, nil
	}
	if err != nil {
		return tenant.Principal{}, false, err
	}
	if p.Kind != tenant.Agent {
		return tenant.Principal{}, false, nil
	}
	return p, true, nil
}

type keyRecord struct {
	ID          string
	PrincipalID string
	Name        string
	Prefix      string
	Scopes      []string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
	Token       string
}

func (m *Module) createAgentKey(ctx context.Context, p tenant.Principal, name string, scopes []string, expires *time.Time) (keyRecord, error) {
	if scopes == nil {
		scopes = []string{}
	}
	var rec keyRecord
	err := m.inTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var principalID string
		err := tx.QueryRow(ctx, `
			SELECT id::text FROM principals
			WHERE kind = 'agent' AND name = $1
			ORDER BY created_at
			LIMIT 1
		`, name).Scan(&principalID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO principals (tenant_id, kind, name, roles)
				VALUES ($1::uuid, 'agent', $2, '{}')
				RETURNING id::text
			`, p.TenantID, name).Scan(&principalID)
		}
		if err != nil {
			return err
		}
		for attempt := 0; attempt < 5; attempt++ {
			prefix, err := prefixFor(p.TenantID)
			if err != nil {
				return err
			}
			secret, hash, err := newSecret()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "SAVEPOINT key_insert"); err != nil {
				return err
			}
			var id string
			var created time.Time
			err = tx.QueryRow(ctx, `
				INSERT INTO agent_keys (tenant_id, principal_id, name, prefix, hash, scopes, expires_at)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
				RETURNING id::text, created_at
			`, p.TenantID, principalID, name, prefix, hash, scopes, expires).Scan(&id, &created)
			if isUnique(err) {
				if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT key_insert"); rbErr != nil {
					return rbErr
				}
				continue
			}
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT key_insert"); err != nil {
				return err
			}
			rec = keyRecord{
				ID:          id,
				PrincipalID: principalID,
				Name:        name,
				Prefix:      prefix,
				Scopes:      scopes,
				CreatedAt:   created,
				ExpiresAt:   expires,
				Token:       "aeon_" + prefix + "_" + secret,
			}
			return nil
		}
		return errors.New("agent key prefix collision")
	})
	return rec, err
}

func (m *Module) listAgentKeys(ctx context.Context, tenantID string) ([]keyRecord, error) {
	var out []keyRecord
	err := m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, principal_id::text, name, prefix, scopes, created_at, expires_at, last_used_at, revoked_at
			FROM agent_keys
			ORDER BY created_at DESC, id
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rec keyRecord
			var scopes pgtype.FlatArray[string]
			if err := rows.Scan(&rec.ID, &rec.PrincipalID, &rec.Name, &rec.Prefix, &scopes, &rec.CreatedAt, &rec.ExpiresAt, &rec.LastUsedAt, &rec.RevokedAt); err != nil {
				return err
			}
			rec.Scopes = []string(scopes)
			if rec.Scopes == nil {
				rec.Scopes = []string{}
			}
			out = append(out, rec)
		}
		return rows.Err()
	})
	if out == nil {
		out = []keyRecord{}
	}
	return out, err
}

func (m *Module) revokeAgentKey(ctx context.Context, tenantID, id string) error {
	return m.inTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE agent_keys
			SET revoked_at = COALESCE(revoked_at, now())
			WHERE id = $1::uuid
		`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return nil
	})
}

type meView struct {
	Email     *string
	Principal tenant.Principal
	TenantID  string
	Slug      string
	Name      string
	Identity  *identityView
}

type identityView struct {
	ID          string
	Issuer      string
	Subject     string
	Email       *string
	DisplayName *string
}

func (m *Module) loadMe(ctx context.Context, p tenant.Principal) (meView, error) {
	view := meView{Principal: p, TenantID: p.TenantID}
	err := m.inTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id::text,slug,name FROM tenants WHERE id=$1::uuid`, p.TenantID).Scan(&view.TenantID, &view.Slug, &view.Name); err != nil {
			return err
		}
		fresh, err := displayPrincipal(ctx, tx, p.TenantID, p.ID)
		if err != nil {
			return err
		}
		view.Principal = fresh
		var iid, issuer, subject *string
		var email, display *string
		err = tx.QueryRow(ctx, `
			SELECT i.id::text, i.issuer, i.subject,
			    COALESCE(NULLIF(profile.email,''),profile_identity.email),
			    CASE WHEN pr.linked_to IS NOT NULL THEN profile.name ELSE i.display_name END
			FROM principals pr
			JOIN principals profile ON profile.tenant_id=pr.tenant_id
			    AND profile.id=COALESCE(pr.linked_to,pr.id)
			LEFT JOIN identities i ON i.id=pr.identity_id
			LEFT JOIN identities profile_identity ON profile_identity.id=profile.identity_id
			WHERE pr.tenant_id=$2::uuid AND pr.id=$1::uuid
		`, p.ID, p.TenantID).Scan(&iid, &issuer, &subject, &email, &display)
		if err != nil {
			return err
		}
		view.Email = email
		if iid != nil {
			view.Identity = &identityView{
				ID:          *iid,
				Issuer:      deref(issuer),
				Subject:     deref(subject),
				Email:       email,
				DisplayName: display,
			}
		}
		return nil
	})
	return view, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Links change presentation only: retain the authenticated ID, kind and roles.
// B4 enforces one-hop, same-tenant links to person targets.
func displayPrincipal(ctx context.Context, tx pgx.Tx, tenantID, id string) (tenant.Principal, error) {
	return scanPrincipal(tx.QueryRow(ctx, `
		SELECT p.id::text,p.tenant_id::text,p.kind,COALESCE(target.name,p.name),p.roles
		FROM principals p
		LEFT JOIN principals target ON target.tenant_id=p.tenant_id AND target.id=p.linked_to
		WHERE p.tenant_id=$1::uuid AND p.id=$2::uuid
	`, tenantID, id))
}
