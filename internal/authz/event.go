// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// appendEvent uses the same events insertion contract as events.Append. Keeping
// this small writer here lets the event undo endpoint call RequireTx without an
// import cycle; all role mutations still write in their tenant transaction.
func appendEvent(ctx context.Context, tx pgx.Tx, actor tenant.Principal, typ string, before, after any) error {
	encode := func(value any) ([]byte, error) {
		if value == nil {
			return nil, nil
		}
		out, err := json.Marshal(value)
		if string(out) == "null" {
			return nil, err
		}
		return out, err
	}
	old, err := encode(before)
	if err != nil {
		return err
	}
	next, err := encode(after)
	if err != nil {
		return err
	}
	if old == nil && next == nil {
		return errors.New("authorization event needs a snapshot")
	}
	_, err = tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,type,before,after,at)
		VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5::jsonb,clock_timestamp())`, actor.TenantID, actor.ID, typ, old, next)
	return err
}
