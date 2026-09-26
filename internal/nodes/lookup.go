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

// lookupKeyPattern and lookupKeyMax are the stored key shape and length
// (nodes.key and node_key_aliases.key); keys are matched upper-cased.
var lookupKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-[1-9][0-9]*$`)

const (
	lookupKeyMax = 30
	// lookupLimit bounds the raw inputs of each list, duplicates included.
	lookupLimit = 100
)

// lookupParts splits repeated and comma-separated values, refusing more than
// lookupLimit raw parts before any of them is parsed.
func lookupParts(values []string) ([]string, bool) {
	parts := make([]string, 0, lookupLimit)
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			if len(parts) == lookupLimit {
				return nil, false
			}
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	return parts, true
}

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
	idParts, ok := lookupParts(values)
	if !ok {
		writeErr(w, badRequest("too many ids"))
		return
	}
	keyParts, ok := lookupParts(keyValues)
	if !ok {
		writeErr(w, badRequest("too many keys"))
		return
	}
	ids := make([]string, 0, len(idParts))
	seen := make(map[string]bool)
	for _, part := range idParts {
		id, valid := parseUUID(part)
		if !valid {
			writeErr(w, badRequest("invalid ids"))
			return
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	keys := make([]string, 0, len(keyParts))
	for _, part := range keyParts {
		key := strings.ToUpper(part)
		if len(key) > lookupKeyMax || !lookupKeyPattern.MatchString(key) {
			writeErr(w, badRequest("invalid keys"))
			return
		}
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
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
