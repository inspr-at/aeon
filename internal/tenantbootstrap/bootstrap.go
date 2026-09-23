// SPDX-License-Identifier: AGPL-3.0-only

// Package tenantbootstrap provides operator-only tenant creation and OIDC
// principal binding. The coordinator can call Create and BindOIDC from CLI
// commands; neither function exposes an HTTP route or contacts an IdP.
package tenantbootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LookupTenantID is a sentinel used only for global directory and session
// lookups before a caller's tenant is known.
const LookupTenantID = "00000000-0000-0000-0000-000000000000"

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ResolveSlug reads the global tenant directory through db.InTenant. The
// zero UUID is used only for this directory lookup, never for tenant rows.
func ResolveSlug(ctx context.Context, pool *pgxpool.Pool, slug string) (string, error) {
	if pool == nil || !slugPattern.MatchString(slug) {
		return "", errors.New("valid tenant slug and database pool are required")
	}
	var id string
	err := db.InTenant(ctx, pool, LookupTenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug=$1`, slug).Scan(&id)
	})
	return id, err
}

// Create makes a tenant with an auditable bootstrap actor. The UUID is
// allocated before entering db.InTenant, so the kind-seeding trigger observes
// the correct tenant setting. Reusing a slug is an error.
func Create(ctx context.Context, pool *pgxpool.Pool, slug, name string) (string, error) {
	name = strings.TrimSpace(name)
	if pool == nil || !slugPattern.MatchString(slug) || name == "" {
		return "", errors.New("valid tenant slug, name and database pool are required")
	}
	id, err := randomUUID()
	if err != nil {
		return "", err
	}
	err = db.InTenant(ctx, pool, id, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1::uuid,$2,$3)`, id, slug, name); err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}
		actor, err := bootstrapActor(ctx, tx, id)
		if err != nil {
			return err
		}
		_, err = events.Append(ctx, tx, tenant.Principal{ID: actor, TenantID: id}, events.Change{
			Type: "tenant.created", After: map[string]any{"id": id, "slug": slug, "name": name},
		})
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// BindOIDC provisions a person in a selected tenant by issuer and subject.
// Existing bindings are unchanged on replay; a different role or name is an
// explicit update with an event. Email never participates in membership.
func BindOIDC(ctx context.Context, pool *pgxpool.Pool, slug, issuer, subject, name, role string) (string, error) {
	issuer, subject, name = strings.TrimSpace(issuer), strings.TrimSpace(subject), strings.TrimSpace(name)
	if issuer == "" || subject == "" || name == "" || (role != "admin" && role != "member" && role != "customer") {
		return "", errors.New("issuer, subject, name and a valid role are required")
	}
	id, err := ResolveSlug(ctx, pool, slug)
	if err != nil {
		return "", fmt.Errorf("resolve tenant: %w", err)
	}
	var principalID string
	err = db.InTenant(ctx, pool, id, func(tx pgx.Tx) error {
		actor, err := bootstrapActor(ctx, tx, id)
		if err != nil {
			return err
		}
		// After the first operator is bound, attribute later CLI changes to
		// that mapped tenant admin instead of the bootstrap actor.
		var operatorID string
		err = tx.QueryRow(ctx, `SELECT p.id::text FROM principals p
			JOIN identities i ON i.id=p.identity_id
			WHERE p.tenant_id=$1::uuid AND p.kind='person'
			  AND 'admin'=ANY(p.roles) AND i.issuer=$2
			ORDER BY p.created_at,p.id LIMIT 1`, id, issuer).Scan(&operatorID)
		if err == nil {
			actor = operatorID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var identityID string
		if _, err := tx.Exec(ctx, `INSERT INTO identities(issuer,subject,display_name)
			VALUES($1,$2,$3) ON CONFLICT(issuer,subject) DO NOTHING`, issuer, subject, name); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&identityID); err != nil {
			return err
		}
		var oldName string
		var oldRoles []string
		err = tx.QueryRow(ctx, `SELECT id::text,name,roles FROM principals
			WHERE tenant_id=$1::uuid AND identity_id=$2::uuid AND kind='person' FOR UPDATE`, id, identityID).Scan(&principalID, &oldName, &oldRoles)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		before := any(nil)
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,identity_id,name,roles)
				VALUES($1::uuid,'person',$2::uuid,$3,ARRAY[$4]::text[]) RETURNING id::text`, id, identityID, name, role).Scan(&principalID); err != nil {
				return err
			}
		} else {
			if oldName == name && len(oldRoles) == 1 && oldRoles[0] == role {
				return nil
			}
			before = map[string]any{"principal_id": principalID, "name": oldName, "roles": oldRoles}
			if _, err := tx.Exec(ctx, `UPDATE principals SET name=$3,roles=ARRAY[$4]::text[]
				WHERE tenant_id=$1::uuid AND id=$2::uuid`, id, principalID, name, role); err != nil {
				return err
			}
		}
		_, err = events.Append(ctx, tx, tenant.Principal{ID: actor, TenantID: id}, events.Change{
			Type: "tenant.principal_bound", Before: before,
			After: map[string]any{"principal_id": principalID, "issuer": issuer, "subject": subject, "name": name, "roles": []string{role}},
		})
		return err
	})
	return principalID, err
}

func bootstrapActor(ctx context.Context, tx pgx.Tx, tenantID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE tenant_id=$1::uuid
		AND kind='agent' AND name='Tenant bootstrap' ORDER BY created_at,id LIMIT 1`, tenantID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles)
			VALUES($1::uuid,'agent','Tenant bootstrap',ARRAY['operator']) RETURNING id::text`, tenantID).Scan(&id)
	}
	return id, err
}

func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
