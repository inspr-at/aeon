// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Keep unknown tenant-defined states verbatim. Only known spelling variants
// are normalized; project states (active/frozen/deleted) retain their meaning.
func canonicalState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "in-progress", "in progress", "in_progress", "inprogress":
		return "in_progress"
	case "canceled", "cancelled":
		return "cancelled"
	case "new", "backlog", "qa", "done", "delivered", "accepted", "archived", "open", "active", "frozen", "deleted":
		return strings.ToLower(strings.TrimSpace(state))
	case "":
		return "open"
	default:
		return state
	}
}

func userDisplayName(u Record) string {
	for _, key := range []string{"display_name", "full_name", "name"} {
		if name := strings.TrimSpace(stringField(u, key)); name != "" {
			return name
		}
	}
	if name := strings.TrimSpace(stringField(u, "first_name") + " " + stringField(u, "last_name")); name != "" {
		return name
	}
	return strings.TrimSpace(stringField(u, "username"))
}

func storedUser(u Record, source string) Record {
	out := Record{"source_id": source}
	for _, key := range []string{"id", "username", "display_name", "full_name", "name", "first_name", "last_name"} {
		if value, ok := u[key]; ok {
			out[key] = value
		}
	}
	return out
}

// identities.subject is the durable source+classic-ID mapping. Read it even
// when a partial snapshot omits users; never match IDs across source instances.
func storedUserRefs(ctx context.Context, tx pgx.Tx, tenantID, source string) (map[int64]string, error) {
	rows, err := tx.Query(ctx, `SELECT i.subject,p.id::text FROM principals p
 JOIN identities i ON i.id=p.identity_id
 WHERE p.tenant_id=$1 AND i.issuer='paimos-classic'
 AND starts_with(i.subject,$2)`, tenantID, source+":")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := map[int64]string{}
	for rows.Next() {
		var subject, id string
		if err := rows.Scan(&subject, &id); err != nil {
			return nil, err
		}
		classicID, err := strconv.ParseInt(strings.TrimPrefix(subject, source+":"), 10, 64)
		if err == nil {
			users[classicID] = id
		}
	}
	return users, rows.Err()
}
