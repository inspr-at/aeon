// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type nodePage struct {
	Items      []nodeJSON `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type treeEntry struct {
	Node  nodeJSON `json:"node"`
	Depth int      `json:"depth"`
}

type treePage struct {
	Items      []treeEntry `json:"items"`
	NextCursor *string     `json:"next_cursor"`
}

type listQuery struct {
	KindID      *string
	State       *string
	ParentSet   bool
	ParentID    *string
	Descendants bool
	Sort        string
	Direction   string
	Limit       int
	Cursor      string
}

type treeQuery struct {
	RootID   *string
	DepthSet bool
	MaxDepth int
	Limit    int
	Cursor   string
}

var sortCols = map[string]string{
	"position":   "n.position",
	"updated_at": "n.updated_at",
	"created_at": "n.created_at",
	"key":        "n.key",
	"title":      "n.title",
}

func (m *Module) handleListNodes(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	q, err := parseListQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	page, err := m.listNodes(r.Context(), p.TenantID, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (m *Module) handleTree(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	q, err := parseTreeQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	page, err := m.nodeTree(r.Context(), p.TenantID, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func parseListQuery(r *http.Request) (listQuery, error) {
	q := r.URL.Query()
	out := listQuery{Sort: "position", Direction: "asc", Limit: 50}
	if q.Has("kind_id") {
		id, ok := parseUUID(q.Get("kind_id"))
		if !ok {
			return listQuery{}, badRequest("invalid kind_id")
		}
		out.KindID = &id
	}
	if q.Has("state") {
		state := q.Get("state")
		if !nonBlank(state) {
			return listQuery{}, badRequest("invalid state")
		}
		out.State = &state
	}
	if q.Has("parent_id") {
		id, ok := parseUUID(q.Get("parent_id"))
		if !ok {
			return listQuery{}, badRequest("invalid parent_id")
		}
		out.ParentSet = true
		out.ParentID = &id
	}
	if q.Has("include_descendants") {
		switch q.Get("include_descendants") {
		case "true":
			out.Descendants = true
		case "false":
			out.Descendants = false
		default:
			return listQuery{}, badRequest("invalid include_descendants")
		}
	}
	if out.Descendants && !out.ParentSet {
		return listQuery{}, badRequest("include_descendants requires parent_id")
	}
	if q.Has("sort") {
		sort := q.Get("sort")
		if _, ok := sortCols[sort]; !ok {
			return listQuery{}, badRequest("invalid sort")
		}
		out.Sort = sort
	}
	if q.Has("direction") {
		switch q.Get("direction") {
		case "asc", "desc":
			out.Direction = q.Get("direction")
		default:
			return listQuery{}, badRequest("invalid direction")
		}
	}
	limit, err := parseLimit(q.Get("limit"), q.Has("limit"))
	if err != nil {
		return listQuery{}, err
	}
	out.Limit = limit
	out.Cursor = q.Get("cursor")
	return out, nil
}

func parseTreeQuery(r *http.Request) (treeQuery, error) {
	q := r.URL.Query()
	out := treeQuery{Limit: 50}
	if q.Has("root_id") {
		id, ok := parseUUID(q.Get("root_id"))
		if !ok {
			return treeQuery{}, badRequest("invalid root_id")
		}
		out.RootID = &id
	}
	if q.Has("max_depth") {
		n, err := strconv.Atoi(q.Get("max_depth"))
		if err != nil || n < 0 {
			return treeQuery{}, badRequest("invalid max_depth")
		}
		out.DepthSet = true
		out.MaxDepth = n
	}
	limit, err := parseLimit(q.Get("limit"), q.Has("limit"))
	if err != nil {
		return treeQuery{}, err
	}
	out.Limit = limit
	out.Cursor = q.Get("cursor")
	return out, nil
}

func parseLimit(raw string, present bool) (int, error) {
	if !present || raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 200 {
		return 0, badRequest("invalid limit")
	}
	return n, nil
}

func (m *Module) listNodes(ctx context.Context, tenantID string, q listQuery) (nodePage, error) {
	var mark *listMark
	if q.Cursor != "" {
		env, err := openCursor(q.Cursor)
		if err != nil {
			return nodePage{}, err
		}
		if env.T != "list" {
			return nodePage{}, badRequest("cursor does not match this query")
		}
		var decoded listMark
		if err := json.Unmarshal(env.P, &decoded); err != nil {
			return nodePage{}, badRequest("invalid cursor")
		}
		if !listMarkMatches(decoded, q) {
			return nodePage{}, badRequest("cursor does not match this query")
		}
		if _, ok := parseUUID(decoded.ID); !ok || decoded.Value == "" {
			return nodePage{}, badRequest("invalid cursor")
		}
		mark = &decoded
	}
	page := nodePage{Items: []nodeJSON{}}
	err := m.tx(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		sql, args := listSQL(q, mark)
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return dbErr("list nodes", err)
		}
		defer rows.Close()
		var items []nodeJSON
		for rows.Next() {
			node, err := scanNode(rows)
			if err != nil {
				return err
			}
			items = append(items, node)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) > q.Limit {
			last := items[q.Limit-1]
			items = items[:q.Limit]
			encoded, err := encodeTyped("list", listMark{
				KindID: q.KindID, State: q.State, ParentSet: q.ParentSet, ParentID: q.ParentID,
				Descendants: q.Descendants, Sort: q.Sort, Direction: q.Direction,
				Value: cursorValue(last, q.Sort), ID: last.ID,
			})
			if err != nil {
				return err
			}
			page.NextCursor = &encoded
		}
		if items == nil {
			items = []nodeJSON{}
		}
		page.Items = items
		return nil
	})
	return page, err
}

func listMarkMatches(mark listMark, q listQuery) bool {
	return sameString(mark.KindID, q.KindID) &&
		sameString(mark.State, q.State) &&
		mark.ParentSet == q.ParentSet &&
		sameString(mark.ParentID, q.ParentID) &&
		mark.Descendants == q.Descendants &&
		mark.Sort == q.Sort &&
		mark.Direction == q.Direction
}

func listSQL(q listQuery, mark *listMark) (string, []any) {
	args := []any{q.ParentSet, q.ParentID, q.Descendants, q.KindID, q.State}
	var b strings.Builder
	b.WriteString(`
		WITH RECURSIVE scope AS (
			SELECT n.id
			FROM nodes n
			WHERE n.deleted_at IS NULL
			  AND ($1::bool = false OR n.parent_id IS NOT DISTINCT FROM $2::uuid)
			UNION ALL
			SELECT c.id
			FROM nodes c
			JOIN scope s ON c.parent_id = s.id
			WHERE $3::bool = true AND c.deleted_at IS NULL
		)
		SELECT ` + nodeCols + `
		FROM nodes n
		WHERE n.deleted_at IS NULL
		  AND n.id IN (SELECT id FROM scope)
		  AND ($4::uuid IS NULL OR n.kind_id = $4::uuid)
		  AND ($5::text IS NULL OR n.state = $5::text)`)
	if mark != nil {
		col := sortCols[q.Sort]
		op := ">"
		if q.Direction == "desc" {
			op = "<"
		}
		cast := "::text"
		switch q.Sort {
		case "position":
			cast = "::numeric"
		case "updated_at", "created_at":
			cast = "::timestamptz"
		}
		args = append(args, mark.Value, mark.ID)
		v := len(args) - 1
		id := len(args)
		fmt.Fprintf(&b, ` AND (%s %s $%d%s OR (%s = $%d%s AND n.id %s $%d::uuid))`,
			col, op, v, cast, col, v, cast, op, id)
	}
	dir := "ASC"
	if q.Direction == "desc" {
		dir = "DESC"
	}
	args = append(args, q.Limit+1)
	fmt.Fprintf(&b, " ORDER BY %s %s, n.id %s LIMIT $%d", sortCols[q.Sort], dir, dir, len(args))
	return b.String(), args
}

func (m *Module) nodeTree(ctx context.Context, tenantID string, q treeQuery) (treePage, error) {
	var mark *treeMark
	if q.Cursor != "" {
		env, err := openCursor(q.Cursor)
		if err != nil {
			return treePage{}, err
		}
		if env.T != "tree" {
			return treePage{}, badRequest("cursor does not match this query")
		}
		var decoded treeMark
		if err := json.Unmarshal(env.P, &decoded); err != nil {
			return treePage{}, badRequest("invalid cursor")
		}
		if !sameString(decoded.RootID, q.RootID) || decoded.DepthSet != q.DepthSet || (q.DepthSet && decoded.MaxDepth != q.MaxDepth) {
			return treePage{}, badRequest("cursor does not match this query")
		}
		if len(decoded.PosPath) == 0 || len(decoded.PosPath) != len(decoded.IDPath) {
			return treePage{}, badRequest("invalid cursor")
		}
		mark = &decoded
	}
	page := treePage{Items: []treeEntry{}}
	err := m.tx(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if q.RootID != nil {
			if _, err := loadNode(ctx, tx, *q.RootID, false); err != nil {
				return err
			}
		}
		var depth any
		if q.DepthSet {
			depth = q.MaxDepth
		}
		args := []any{q.RootID, depth}
		var posPath, idPath any
		if mark != nil {
			posPath = mark.PosPath
			idPath = mark.IDPath
		}
		args = append(args, posPath, idPath, q.Limit+1)
		rows, err := tx.Query(ctx, `
			WITH RECURSIVE walk AS (
				SELECT n.id, 0 AS depth,
					ARRAY[n.position]::numeric[] AS pos_path,
					ARRAY[n.id]::uuid[] AS id_path
				FROM nodes n
				WHERE n.deleted_at IS NULL
				  AND (($1::uuid IS NULL AND n.parent_id IS NULL) OR n.id = $1::uuid)
				UNION ALL
				SELECT c.id, w.depth + 1, w.pos_path || c.position, w.id_path || c.id
				FROM nodes c
				JOIN walk w ON c.parent_id = w.id
				WHERE c.deleted_at IS NULL
				  AND ($2::int IS NULL OR w.depth < $2::int)
			)
			SELECT `+nodeCols+`, w.depth, w.pos_path::text[], w.id_path::text[]
			FROM walk w
			JOIN nodes n ON n.id = w.id
			WHERE $3::text[] IS NULL
			   OR (w.pos_path, w.id_path) > ($3::text[]::numeric[], $4::text[]::uuid[])
			ORDER BY w.pos_path, w.id_path
			LIMIT $5`, args...)
		if err != nil {
			return dbErr("node tree", err)
		}
		defer rows.Close()
		type walked struct {
			node    nodeJSON
			depth   int
			posPath []string
			idPath  []string
		}
		var items []walked
		for rows.Next() {
			var item walked
			var fields, position string
			if err := rows.Scan(
				&item.node.ID, &item.node.Key, &item.node.KindID, &item.node.Title, &item.node.Body,
				&fields, &item.node.State, &item.node.ParentID, &position,
				&item.node.CreatedAt, &item.node.UpdatedAt, &item.node.DeletedAt,
				&item.depth, &item.posPath, &item.idPath,
			); err != nil {
				return err
			}
			if fields == "" {
				fields = "{}"
			}
			item.node.Fields = json.RawMessage(fields)
			item.node.Position = trimDecimal(position)
			item.posPath = trimPath(item.posPath)
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) > q.Limit {
			last := items[q.Limit-1]
			items = items[:q.Limit]
			encoded, err := encodeTyped("tree", treeMark{
				RootID: q.RootID, DepthSet: q.DepthSet, MaxDepth: q.MaxDepth,
				PosPath: last.posPath, IDPath: last.idPath,
			})
			if err != nil {
				return err
			}
			page.NextCursor = &encoded
		}
		page.Items = make([]treeEntry, len(items))
		for i, item := range items {
			page.Items[i] = treeEntry{Node: item.node, Depth: item.depth}
		}
		return nil
	})
	return page, err
}

func trimPath(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = trimDecimal(s)
	}
	return out
}
