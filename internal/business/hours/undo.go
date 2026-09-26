// SPDX-License-Identifier: AGPL-3.0-only

package hours

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/business/costunits"
	"github.com/inspr-at/paimos/internal/events"
	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/tenant"
	"github.com/inspr-at/paimos/internal/workorders"
)

// UndoHandlers is passed by the coordinator to events.WithUndoHandlers. With
// no argument it uses the compiled costs/hours manifests; the shared registry
// may instead be supplied explicitly. It never mounts the event API itself.
func UndoHandlers(registries ...*plugins.Registry) map[string]events.UndoFunc {
	var registry *plugins.Registry
	if len(registries) > 0 {
		registry = registries[0]
	} else {
		registry = plugins.NewRegistry()
		for _, build := range []func() (plugins.Plugin, error){costunits.Plugin, Plugin} {
			plug, err := build()
			if err != nil || registry.Register(plug) != nil {
				registry = nil
				break
			}
		}
		if registry != nil {
			registry.Seal()
		}
	}
	m := &Module{registry: registry}
	return map[string]events.UndoFunc{"time_entry.updated": m.undoEntry, "time_entry.deleted": m.undoEntry}
}

func (m *Module) undoEntry(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var before, expected Entry
	if json.Unmarshal(e.Before, &before) != nil || !workorders.UUID(before.ID) || !workorders.UUID(before.PeriodID) {
		return events.Change{}, events.ErrConflict
	}
	p, err := refreshPerson(ctx, tx, p)
	if err != nil {
		return events.Change{}, events.ErrForbidden
	}
	if before.PrincipalID != p.ID && !admin(ctx, tx, p) {
		return events.Change{}, events.ErrForbidden
	}
	// Also enforce current event-actor authority, rather than a stale role claim
	// accepted by the generic event endpoint.
	if e.ActorPrincipalID != p.ID && !admin(ctx, tx, p) {
		return events.Change{}, events.ErrForbidden
	}
	if err = m.gate(ctx, tx, p, "time_entry"); err != nil {
		return events.Change{}, events.ErrForbidden
	}
	period, err := loadPeriod(ctx, tx, before.PeriodID)
	if err != nil {
		return events.Change{}, err
	}
	if period.State != "open" {
		return events.Change{}, events.ErrConflict
	}
	current, err := scanEntry(tx.QueryRow(ctx, `SELECT `+entryColumns+` FROM time_entries WHERE id=$1 FOR UPDATE`, before.ID))
	change := events.Change{NodeID: &before.NodeID, Type: e.Type}
	var restored Entry
	switch e.Type {
	case "time_entry.updated":
		if err != nil {
			return change, err
		}
		if json.Unmarshal(e.After, &expected) != nil || !reflect.DeepEqual(comparable(current), comparable(expected)) {
			return change, events.ErrConflict
		}
		restored, err = updateEntry(ctx, tx, before)
		change.Before = current
	case "time_entry.deleted":
		if err == nil {
			return change, events.ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return change, err
		}
		if len(e.After) > 0 && string(e.After) != "null" {
			return change, events.ErrConflict
		}
		restored, err = scanEntry(tx.QueryRow(ctx, `INSERT INTO time_entries(tenant_id,id,period_id,principal_id,node_id,cost_unit_node_id,source,agent_run_id,started_at,ended_at,duration_seconds,rate_amount,currency,amount,note,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::numeric,$13,$14::numeric,$15,GREATEST(clock_timestamp(),$16::timestamptz+interval '1 microsecond')) RETURNING `+entryColumns, p.TenantID, before.ID, before.PeriodID, before.PrincipalID, before.NodeID, before.CostUnitID, before.Source, before.AgentRunID, before.StartedAt, before.EndedAt, before.DurationSeconds, before.RateAmount.String(), before.Currency, before.Amount.String(), before.Note, before.UpdatedAt))
	default:
		return change, events.ErrConflict
	}
	if err != nil {
		return change, err
	}
	if err = bumpPeriod(ctx, tx, period.ID); err != nil {
		return change, err
	}
	change.After = restored
	return change, nil
}

// comparable normalises persisted and JSON-decoded timestamps to the same
// UTC, microsecond representation, independent of the database/host time zone.
func comparable(e Entry) Entry {
	e.StartedAt = e.StartedAt.UTC().Truncate(time.Microsecond)
	e.EndedAt = e.EndedAt.UTC().Truncate(time.Microsecond)
	e.UpdatedAt = e.UpdatedAt.UTC().Truncate(time.Microsecond)
	return e
}
