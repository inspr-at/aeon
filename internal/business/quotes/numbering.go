// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var legacyCustomerNumber = regexp.MustCompile(`^K[0-9]{2}-[0-9]{3,}$`)

func quoteDay(s quoteSettings, now time.Time) (time.Time, error) {
	if s.Revision == 0 {
		return time.Time{}, bad("quote settings must be configured")
	}
	loc, err := time.LoadLocation(s.NumberingTimeZone)
	if err != nil {
		return time.Time{}, err
	}
	return now.In(loc), nil
}
func nextNumber(ctx context.Context, tx pgx.Tx, tenantID, kind, period string) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `INSERT INTO quote_number_sequences(tenant_id,kind,period,value) VALUES($1::uuid,$2,$3,1) ON CONFLICT(tenant_id,kind,period) DO UPDATE SET value=quote_number_sequences.value+1 RETURNING value`, tenantID, kind, period).Scan(&n)
	return n, err
}
func ensureCustomerNumber(ctx context.Context, tx pgx.Tx, p tenant.Principal, org string, day time.Time) (string, error) {
	// All first-offer writers for one organisation take this lock before
	// allocating a monthly sequence. The sequence row handles distinct orgs.
	var locked string
	err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE id=$1::uuid AND deleted_at IS NULL FOR UPDATE`, org).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", missing()
	}
	if err != nil {
		return "", err
	}
	var number string
	err = tx.QueryRow(ctx, `SELECT customer_no FROM crm_customer_numbers WHERE organisation_node_id=$1::uuid`, org).Scan(&number)
	if err == nil {
		return number, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	period := day.Format("0601")
	n, err := nextNumber(ctx, tx, p.TenantID, "customer", period)
	if err != nil {
		return "", err
	}
	number = fmt.Sprintf("K%s%d", period, n)
	_, err = tx.Exec(ctx, `INSERT INTO crm_customer_numbers(tenant_id,organisation_node_id,customer_no) VALUES($1::uuid,$2::uuid,$3)`, p.TenantID, org, number)
	if err != nil {
		return "", err
	}
	if err = appendEvent(ctx, tx, p, org, "crm.customer_number_allocated", nil, map[string]any{"organisation_node_id": org, "customer_no": number}); err != nil {
		return "", err
	}
	return number, nil
}
func allocateOfferNumber(ctx context.Context, tx pgx.Tx, p tenant.Principal, day time.Time) (string, error) {
	period := day.Format("060102")
	n, err := nextNumber(ctx, tx, p.TenantID, "offer", period)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("A%s-%02d", period, n), nil
}

// ReformatLegacyCustomerNumber is the guarded domain operation used by the
// CRM package's admin adapter. It never changes an issued quote or history.
func ReformatLegacyCustomerNumber(ctx context.Context, pool *pgxpool.Pool, registry *plugins.Registry, p tenant.Principal, org, expected string) (string, error) {
	if pool == nil || registry == nil || authz.Require(authz.BindPool(tenant.WithPrincipal(ctx, p), pool), "quotes.manage", authz.Scope{}) != nil {
		return "", denied()
	}
	if !uuidRe.MatchString(org) || !legacyCustomerNumber.MatchString(expected) {
		return "", bad("invalid legacy customer number")
	}
	m := &Module{pool: pool, registry: registry}
	var result string
	err := db.InTenant(tenant.WithPrincipal(ctx, p), pool, p.TenantID, func(tx pgx.Tx) error {
		if err := m.enabled(ctx, tx, p.TenantID, fence.PermNodesContribute, true); err != nil {
			return err
		}
		var live bool
		if err := tx.QueryRow(ctx, `SELECT aeon_business_node_kind($1::uuid,$2::uuid,'organisation')`, p.TenantID, org).Scan(&live); err != nil {
			return err
		}
		if !live {
			return missing()
		}
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE id=$1::uuid FOR UPDATE`, org).Scan(&locked); err != nil {
			return err
		}
		var current string
		if err := tx.QueryRow(ctx, `SELECT customer_no FROM crm_customer_numbers WHERE organisation_node_id=$1::uuid FOR UPDATE`, org).Scan(&current); err != nil {
			return err
		}
		if current != expected {
			return conflict("customer number changed")
		}
		rows, err := tx.Query(ctx, `SELECT quote_node_id::text,state,revision FROM business_quotes WHERE customer_org_node_id=$1::uuid ORDER BY quote_node_id FOR UPDATE`, org)
		if err != nil {
			return err
		}
		type target struct {
			id, state string
			revision  int64
		}
		var targets []target
		for rows.Next() {
			var q target
			if err := rows.Scan(&q.id, &q.state, &q.revision); err != nil {
				rows.Close()
				return err
			}
			targets = append(targets, q)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, q := range targets {
			if q.state != "draft" {
				return conflict("issued quote fixes customer number")
			}
		}
		settings, err := readSettings(ctx, tx)
		if err != nil {
			return err
		}
		day, err := quoteDay(settings, time.Now())
		if err != nil {
			return err
		}
		n, err := nextNumber(ctx, tx, p.TenantID, "customer", day.Format("0601"))
		if err != nil {
			return err
		}
		result = fmt.Sprintf("K%s%d", day.Format("0601"), n)
		_, err = tx.Exec(ctx, `UPDATE crm_customer_numbers SET customer_no=$1,provenance='converted' WHERE organisation_node_id=$2::uuid`, result, org)
		if err != nil {
			return err
		}
		for _, q := range targets {
			var revision int64
			err = tx.QueryRow(ctx, `UPDATE quote_drafts SET document=jsonb_set(document,'{recipient,customer_no}',to_jsonb($1::text),true),draft_revision=draft_revision+1,updated_at=clock_timestamp(),updated_by_principal_id=$2::uuid WHERE quote_node_id=$3::uuid RETURNING draft_revision`, result, p.ID, q.id).Scan(&revision)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE business_quotes SET revision=revision+1 WHERE quote_node_id=$1::uuid`, q.id)
			if err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, p, q.id, "quote.customer_number_changed", map[string]any{"revision": q.revision}, map[string]any{"revision": q.revision + 1, "draft_revision": revision}); err != nil {
				return err
			}
		}
		return appendEvent(ctx, tx, p, org, "crm.customer_number_converted", map[string]any{"customer_no": expected}, map[string]any{"customer_no": result})
	})
	return result, err
}
