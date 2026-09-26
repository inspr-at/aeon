// SPDX-License-Identifier: AGPL-3.0-only

package costunits

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/events"
	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
	"github.com/inspr-at/paimos/internal/tenant"
)

// Rate is one effective-dated price. Amounts JSON-encode as exact numbers.
type Rate struct {
	ID                   string    `json:"id"`
	CostUnitNodeID       string    `json:"cost_unit_node_id"`
	Unit                 string    `json:"unit"`
	Currency             string    `json:"currency"`
	Internal             Decimal   `json:"internal_amount"`
	Bill                 Decimal   `json:"bill_amount"`
	EffectiveFrom        string    `json:"effective_from"`
	EffectiveUntil       *string   `json:"effective_until"`
	CreatedByPrincipalID string    `json:"created_by_principal_id"`
	CreatedAt            time.Time `json:"created_at"`
}

type rateInput struct {
	Unit     string
	Currency string
	Internal Decimal
	Bill     Decimal
	From     string
	Until    string
}

type installPin struct {
	enabled bool
	digest  string
	perms   []string
}

const rateCols = `id::text, cost_unit_node_id::text, unit, currency,
	internal_amount::text, bill_amount::text,
	to_char(effective_from, 'YYYY-MM-DD'), to_char(effective_until, 'YYYY-MM-DD'),
	created_by_principal_id::text, created_at`

func (m *Module) authorize(ctx context.Context, tx pgx.Tx, tenantID, perm string, lock bool) error {
	if err := m.requirePin(ctx, tx, tenantID, perm, lock); err != nil {
		return err
	}
	if perm != fence.PermStepsApply {
		return nil
	}
	if m.reg == nil {
		return plugins.ErrClosed
	}
	ok, err := plugins.Enabled(ctx, tx, m.reg, tenantID, PluginID, StepKey)
	if err != nil {
		return err
	}
	if !ok {
		return plugins.ErrClosed
	}
	return nil
}

func (m *Module) requirePin(ctx context.Context, tx pgx.Tx, tenantID, perm string, lock bool) error {
	if m.reg == nil {
		return plugins.ErrClosed
	}
	plug, ok := m.reg.Lookup(PluginID)
	if !ok || plug.Manifest.DigestSHA256 == "" {
		return plugins.ErrClosed
	}
	query := `SELECT enabled, manifest_digest_sha256, permissions
		FROM plugin_installations
		WHERE tenant_id = $1::uuid AND plugin_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var row installPin
	err := tx.QueryRow(ctx, query, tenantID, PluginID).Scan(&row.enabled, &row.digest, &row.perms)
	if errors.Is(err, pgx.ErrNoRows) {
		return plugins.ErrClosed
	}
	if err != nil {
		return err
	}
	if !row.enabled || row.digest != plug.Manifest.DigestSHA256 {
		return plugins.ErrClosed
	}
	if !slices.Contains(row.perms, perm) {
		return plugins.ErrDenied
	}
	return nil
}

func lockCostUnit(ctx context.Context, tx pgx.Tx, tenantID, nodeID string) error {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT n.id::text
		FROM nodes n
		JOIN node_kinds k ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
		WHERE n.tenant_id = $1::uuid
		  AND n.tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
		  AND n.id = $2::uuid
		  AND k.slug = $3
		  AND n.deleted_at IS NULL
		FOR UPDATE OF n`, tenantID, nodeID, KindSlug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound("cost unit not found")
	}
	return err
}

func costUnitVisible(ctx context.Context, tx pgx.Tx, tenantID, nodeID string) error {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT n.id::text
		FROM nodes n
		JOIN node_kinds k ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
		WHERE n.tenant_id = $1::uuid
		  AND n.tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
		  AND n.id = $2::uuid
		  AND k.slug = $3
		  AND n.deleted_at IS NULL`, tenantID, nodeID, KindSlug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound("cost unit not found")
	}
	return err
}

func listRates(ctx context.Context, tx pgx.Tx, tenantID, nodeID string) ([]Rate, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+rateCols+`
		FROM cost_unit_rates
		WHERE tenant_id = $1::uuid
		  AND tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
		  AND cost_unit_node_id = $2::uuid
		ORDER BY effective_from, unit, currency, id`, tenantID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Rate, 0)
	for rows.Next() {
		rate, err := scanRate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rate)
	}
	return out, rows.Err()
}

func lockRates(ctx context.Context, tx pgx.Tx, tenantID, nodeID, unit, currency string) ([]Rate, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+rateCols+`
		FROM cost_unit_rates
		WHERE tenant_id = $1::uuid
		  AND tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
		  AND cost_unit_node_id = $2::uuid
		  AND unit = $3 AND currency = $4
		ORDER BY effective_from, id
		FOR UPDATE`, tenantID, nodeID, unit, currency)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rate
	for rows.Next() {
		rate, err := scanRate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rate)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRate(row scanner) (Rate, error) {
	var rate Rate
	var internal, bill string
	var until *string
	if err := row.Scan(&rate.ID, &rate.CostUnitNodeID, &rate.Unit, &rate.Currency, &internal, &bill, &rate.EffectiveFrom, &until, &rate.CreatedByPrincipalID, &rate.CreatedAt); err != nil {
		return Rate{}, err
	}
	parsedInternal, err := parseAmountText(internal)
	if err != nil {
		return Rate{}, err
	}
	parsedBill, err := parseAmountText(bill)
	if err != nil {
		return Rate{}, err
	}
	rate.Internal = parsedInternal
	rate.Bill = parsedBill
	rate.EffectiveUntil = until
	return rate, nil
}

func parseAmountText(s string) (Decimal, error) {
	return parseAmount(json.Number(s))
}

func insertRate(ctx context.Context, tx pgx.Tx, p tenant.Principal, nodeID string, in rateInput) (Rate, error) {
	var until any
	if in.Until != "" {
		until = in.Until
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO cost_unit_rates (
			tenant_id, cost_unit_node_id, unit, currency,
			internal_amount, bill_amount, effective_from, effective_until,
			created_by_principal_id
		) VALUES (
			$1::uuid, $2::uuid, $3, $4,
			$5::numeric, $6::numeric, $7::date, $8::date,
			$9::uuid
		)
		RETURNING `+rateCols,
		p.TenantID, nodeID, in.Unit, in.Currency, in.Internal.String(), in.Bill.String(), in.From, until, p.ID)
	return scanRate(row)
}

func closeRate(ctx context.Context, tx pgx.Tx, tenantID, rateID, until string) (Rate, error) {
	row := tx.QueryRow(ctx, `
		UPDATE cost_unit_rates
		SET effective_until = $3::date
		WHERE tenant_id = $1::uuid
		  AND tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid
		  AND id = $2::uuid
		RETURNING `+rateCols, tenantID, rateID, until)
	return scanRate(row)
}

func (m *Module) appendChange(ctx context.Context, tx pgx.Tx, p tenant.Principal, nodeID, typ string, before, after any) error {
	id := nodeID
	_, err := m.appendEvent(ctx, tx, p, events.Change{
		NodeID: &id,
		Type:   typ,
		Before: before,
		After:  after,
	})
	return err
}

// overlaps reports whether two half-open date intervals share a day.
// An empty until means the interval is open.
func overlaps(aFrom, aUntil, bFrom, bUntil string) bool {
	if aUntil != "" && bFrom >= aUntil {
		return false
	}
	if bUntil != "" && aFrom >= bUntil {
		return false
	}
	return true
}

func untilText(until *string) string {
	if until == nil {
		return ""
	}
	return *until
}
