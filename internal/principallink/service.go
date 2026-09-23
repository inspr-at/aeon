// SPDX-License-Identifier: AGPL-3.0-only
package principallink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Person struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Email    *string `json:"email"`
	LinkedTo *string `json:"linked_to"`
}
type Result struct {
	Person  Person `json:"person"`
	Changed bool   `json:"changed"`
}

// Resolve returns a canonical principal within the caller's db.InTenant
// transaction. It preserves agents and never grants the target's roles.
func Resolve(ctx context.Context, tx pgx.Tx, tenantID, id string) (string, string, error) {
	var canonical, name string
	err := tx.QueryRow(ctx, `SELECT coalesce(t.id,p.id)::text,coalesce(t.name,p.name)
 FROM principals p LEFT JOIN principals t ON t.tenant_id=p.tenant_id AND t.id=p.linked_to
 WHERE p.tenant_id=$1 AND p.id=$2 FOR SHARE OF p`, tenantID, id).Scan(&canonical, &name)
	return canonical, name, err
}

func lookup(ctx context.Context, tx pgx.Tx, tenantID, ref string) (Person, error) {
	var p Person
	rows, err := tx.Query(ctx, `SELECT id::text,name,email,linked_to::text FROM principals
 WHERE tenant_id=$1 AND kind='person' AND (id::text=$2 OR name=$2) ORDER BY id LIMIT 2`, tenantID, ref)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.LinkedTo); err != nil {
			return p, err
		}
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	if count == 0 {
		return p, errors.New("person not found in tenant")
	}
	if count != 1 {
		return p, errors.New("ambiguous principal name; use an ID")
	}
	return p, nil
}
func (s *Service) Link(ctx context.Context, slug, from, to string) (Result, error) {
	if strings.TrimSpace(to) == "" {
		return Result{}, errors.New("to is required")
	}
	return s.change(ctx, slug, from, to)
}
func (s *Service) Unlink(ctx context.Context, slug, from string) (Result, error) {
	return s.change(ctx, slug, from, "")
}
func (s *Service) change(ctx context.Context, slug, from, to string) (Result, error) {
	var result Result
	if strings.TrimSpace(from) == "" {
		return result, errors.New("from is required")
	}
	tid, err := tenantbootstrap.ResolveSlug(ctx, s.pool, slug)
	if err != nil {
		return result, err
	}
	err = db.InTenant(ctx, s.pool, tid, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,532))`, tid); err != nil {
			return err
		}
		source, err := lookup(ctx, tx, tid, from)
		if err != nil {
			return err
		}
		var target *string
		if to != "" {
			dest, err := lookup(ctx, tx, tid, to)
			if err != nil {
				return err
			}
			if dest.ID == source.ID || dest.LinkedTo != nil {
				return errors.New("self-links, chains and cycles are forbidden")
			}
			target = &dest.ID
			if source.LinkedTo != nil && *source.LinkedTo != dest.ID {
				return errors.New("principal already linked; unlink first")
			}
		}
		result.Person = source
		if (source.LinkedTo == nil && target == nil) || (source.LinkedTo != nil && target != nil && *source.LinkedTo == *target) {
			return nil
		}
		var before, after json.RawMessage
		if err := tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM principals p WHERE tenant_id=$1 AND id=$2`, tid, source.ID).Scan(&before); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE principals SET linked_to=$3 WHERE tenant_id=$1 AND id=$2 RETURNING to_jsonb(principals)`, tid, source.ID, target).Scan(&after); err != nil {
			return err
		}
		actor, err := operator(ctx, tx, tid)
		if err != nil {
			return err
		}
		typ := "principal.linked"
		if target == nil {
			typ = "principal.unlinked"
		}
		if _, err := events.Append(ctx, tx, tenant.Principal{TenantID: tid, ID: actor}, events.Change{Type: typ, Before: before, After: after}); err != nil {
			return err
		}
		result.Person.LinkedTo = target
		result.Changed = true
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("principal link: %w", err)
	}
	return result, nil
}
func operator(ctx context.Context, tx pgx.Tx, tid string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE tenant_id=$1 AND kind='agent' AND name='Principal link operator' AND roles=ARRAY['operator']::text[] ORDER BY id LIMIT 1`, tid).Scan(&id)
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	var after json.RawMessage
	err = tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1,'agent','Principal link operator',ARRAY['operator']) RETURNING id::text,to_jsonb(principals)`, tid).Scan(&id, &after)
	if err != nil {
		return "", err
	}
	_, err = events.Append(ctx, tx, tenant.Principal{TenantID: tid, ID: id}, events.Change{Type: "principal.created", After: after})
	return id, err
}

type Suggestion struct {
	From   Person `json:"from"`
	To     Person `json:"to"`
	Reason string `json:"reason"`
}

func (s *Service) Suggest(ctx context.Context, slug string) ([]Suggestion, error) {
	out := []Suggestion{}
	tid, err := tenantbootstrap.ResolveSlug(ctx, s.pool, slug)
	if err != nil {
		return nil, err
	}
	err = db.InTenant(ctx, s.pool, tid, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `WITH people AS (
   SELECT p.id,p.name,coalesce(nullif(p.email,''),i.email) email,i.issuer
   FROM principals p JOIN identities i ON i.id=p.identity_id
   WHERE p.tenant_id=$1 AND p.kind='person' AND p.linked_to IS NULL
  ) SELECT a.id::text,a.name,a.email,b.id::text,b.name,b.email,
  CASE WHEN lower(trim(a.email))=lower(trim(b.email)) THEN 'same_email' ELSE 'username_email_local_part' END
  FROM people a JOIN people b ON a.id<>b.id
  WHERE a.issuer='paimos-classic' AND b.issuer<>'paimos-classic'
   AND nullif(trim(b.email),'') IS NOT NULL AND strpos(b.email,'@')>1
   AND (lower(trim(a.email))=lower(trim(b.email)) OR lower(trim(a.name))=lower(split_part(trim(b.email),'@',1)))
  ORDER BY a.name,a.id,b.name,b.id`, tid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v Suggestion
			if err := rows.Scan(&v.From.ID, &v.From.Name, &v.From.Email, &v.To.ID, &v.To.Name, &v.To.Email, &v.Reason); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}
