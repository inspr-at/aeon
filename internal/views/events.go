// SPDX-License-Identifier: AGPL-3.0-only

package views

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// EventWriter appends a resource event using the caller's transaction.
// Implementations must not commit the transaction.
type EventWriter interface {
	Append(context.Context, pgx.Tx, string, string, any, any) error
}

type sqlEventWriter struct{}

func (sqlEventWriter) Append(ctx context.Context, tx pgx.Tx, actorID, eventType string, before, after any) error {
	var beforeJSON, afterJSON []byte
	var err error
	if before != nil {
		beforeJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		afterJSON, err = json.Marshal(after)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO events (tenant_id, actor_principal_id, type, before, after)
		VALUES (NULLIF(current_setting('aeon.tenant_id', true), '')::uuid,
		        $1::uuid, $2, $3::jsonb, $4::jsonb)`,
		actorID, eventType, nullableJSON(beforeJSON), nullableJSON(afterJSON))
	return err
}

func nullableJSON(data []byte) any {
	if data == nil {
		return nil
	}
	return data
}
