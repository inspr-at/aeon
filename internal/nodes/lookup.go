// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type nodePreview struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	State string `json:"state"`
}

// handleLookupNodes resolves relation chips with one bounded query. It returns
// only live nodes visible in the caller's tenant; absent IDs stay absent.
func (m *Module) handleLookupNodes(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	values := r.URL.Query()["ids"]
	if len(values) == 0 {
		writeErr(w, badRequest("ids are required"))
		return
	}
	ids := make([]string, 0, 100)
	seen := make(map[string]bool)
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			id, valid := parseUUID(strings.TrimSpace(part))
			if !valid {
				writeErr(w, badRequest("invalid ids"))
				return
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
				if len(ids) > 100 {
					writeErr(w, badRequest("too many ids"))
					return
				}
			}
		}
	}
	items := make([]nodePreview, 0, len(ids))
	err := m.tx(r.Context(), p.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT n.id::text,n.key,n.title,n.state
			FROM unnest($1::uuid[]) WITH ORDINALITY requested(id,ord)
			JOIN nodes n ON n.tenant_id=current_setting('aeon.tenant_id')::uuid AND n.id=requested.id
			WHERE n.deleted_at IS NULL ORDER BY requested.ord`, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item nodePreview
			if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.State); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	if err != nil {
		writeErr(w, dbErr("lookup nodes", err))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []nodePreview `json:"items"`
	}{items})
}
