// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

type nodePreview struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	State string `json:"state"`
	// Key lookups only: the key that was asked for (a current or earlier key)
	// and the project the node lives in, so a caller can link to it.
	RequestedKey string `json:"requested_key,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
}

// lookupKeyPattern is the node key shape (see node_key_aliases); keys are matched upper-cased.
var lookupKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-[1-9][0-9]*$`)

const lookupLimit = 100

// handleLookupNodes resolves relation chips and ticket keys (release notes)
// with bounded queries. It returns only live nodes visible in the caller's
// tenant; absent IDs and keys stay absent. ID previews come first, then key
// previews, each in request order.
func (m *Module) handleLookupNodes(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	values, keyValues := r.URL.Query()["ids"], r.URL.Query()["keys"]
	if len(values) == 0 && len(keyValues) == 0 {
		writeErr(w, badRequest("ids or keys are required"))
		return
	}
	ids := make([]string, 0, lookupLimit)
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
				if len(ids) > lookupLimit {
					writeErr(w, badRequest("too many ids"))
					return
				}
			}
		}
	}
	keys := make([]string, 0, lookupLimit)
	for _, raw := range keyValues {
		for _, part := range strings.Split(raw, ",") {
			key := strings.ToUpper(strings.TrimSpace(part))
			if !lookupKeyPattern.MatchString(key) {
				writeErr(w, badRequest("invalid keys"))
				return
			}
			if !seen[key] {
				keys = append(keys, key)
				seen[key] = true
				if len(keys) > lookupLimit {
					writeErr(w, badRequest("too many keys"))
					return
				}
			}
		}
	}
	items := make([]nodePreview, 0, len(ids)+len(keys))
	err := m.tx(r.Context(), p.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		if len(ids) > 0 {
			rows, err := tx.Query(ctx, `SELECT n.id::text,n.key,n.title,n.state
			FROM unnest($1::uuid[]) WITH ORDINALITY requested(id,ord)
			JOIN nodes n ON n.tenant_id=current_setting('aeon.tenant_id')::uuid AND n.id=requested.id
			WHERE n.deleted_at IS NULL ORDER BY requested.ord`, ids)
			if err != nil {
				return err
			}
			for rows.Next() {
				var item nodePreview
				if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.State); err != nil {
					rows.Close()
					return err
				}
				items = append(items, item)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		if len(keys) == 0 {
			return nil
		}
		// A current key wins over an earlier key (an alias left by a project move).
		rows, err := tx.Query(ctx, `SELECT requested.key,n.id::text,n.key,n.title,n.state,coalesce(project.id::text,'')
			FROM unnest($1::text[]) WITH ORDINALITY requested(key,ord)
			CROSS JOIN LATERAL (
				SELECT c.id,c.key,c.title,c.state,c.parent_id FROM nodes c
				WHERE c.tenant_id=current_setting('aeon.tenant_id')::uuid AND c.key=requested.key AND c.deleted_at IS NULL
				UNION ALL
				SELECT c.id,c.key,c.title,c.state,c.parent_id FROM node_key_aliases a
				JOIN nodes c ON c.tenant_id=a.tenant_id AND c.id=a.node_id AND c.deleted_at IS NULL
				WHERE a.tenant_id=current_setting('aeon.tenant_id')::uuid AND a.key=requested.key
				LIMIT 1
			) n
			LEFT JOIN LATERAL (
				WITH RECURSIVE ancestors AS (
					SELECT n.id,n.parent_id,0 AS depth
					UNION ALL SELECT a.id,a.parent_id,anc.depth+1 FROM nodes a JOIN ancestors anc ON a.id=anc.parent_id
					WHERE a.tenant_id=current_setting('aeon.tenant_id')::uuid AND a.deleted_at IS NULL AND anc.depth<64
				) SELECT a.id FROM ancestors a JOIN nodes an ON an.id=a.id JOIN node_kinds ak ON ak.id=an.kind_id
				WHERE ak.slug='project' ORDER BY a.depth LIMIT 1
			) project ON true
			ORDER BY requested.ord`, keys)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item nodePreview
			if err := rows.Scan(&item.RequestedKey, &item.ID, &item.Key, &item.Title, &item.State, &item.ProjectID); err != nil {
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
