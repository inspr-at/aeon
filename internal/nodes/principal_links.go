// SPDX-License-Identifier: AGPL-3.0-only
package nodes

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/principallink"
)

// Canonicalize explicit native assignments before persisting and recording the
// node snapshot. Classic source metadata stays verbatim for provenance.
func canonicalAssignments(ctx context.Context, tx pgx.Tx, tenantID string, raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for _, key := range []string{"assignee", "assignee_id"} {
		value, ok := fields[key]
		if !ok || string(value) == "null" {
			continue
		}
		var id string
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &id) != nil {
			if json.Unmarshal(value, &object) != nil || json.Unmarshal(object["id"], &id) != nil {
				return nil, badRequest("invalid assignee")
			}
		}
		if _, ok := parseUUID(id); !ok {
			return nil, badRequest("invalid assignee")
		}
		canonical, name, err := principallink.Resolve(ctx, tx, tenantID, id)
		if err == pgx.ErrNoRows {
			return nil, badRequest("assignee not found in tenant")
		}
		if err != nil {
			return nil, err
		}
		if object != nil {
			object["id"], _ = json.Marshal(canonical)
			if _, ok := object["name"]; ok {
				object["name"], _ = json.Marshal(name)
			}
			fields[key], _ = json.Marshal(object)
		} else {
			fields[key], _ = json.Marshal(canonical)
		}
	}
	return json.Marshal(fields)
}
