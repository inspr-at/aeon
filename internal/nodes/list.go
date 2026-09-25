// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type listPerson struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type listParent struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	KindSlug string `json:"kind_slug"`
}
type listProject struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

// listEpic is the nearest epic above an item: a ticket's own epic, or for a
// task the epic of its ticket. Nil when the item sits under no epic.
type listEpic struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}
type listItem struct {
	nodeJSON
	KindSlug      string       `json:"kind_slug"`
	KindLabel     string       `json:"kind_label"`
	Priority      *string      `json:"priority"`
	Assignee      *listPerson  `json:"assignee"`
	Parent        *listParent  `json:"parent"`
	ChildrenCount int          `json:"children_count"`
	Project       *listProject `json:"project"`
	Epic          *listEpic    `json:"epic"`
}
type nodePage struct {
	Items      []listItem                `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
	Facets     map[string]map[string]int `json:"facets,omitempty"`
}
type treeEntry struct {
	Node  nodeJSON `json:"node"`
	Depth int      `json:"depth"`
}
type treePage struct {
	Items      []treeEntry `json:"items"`
	NextCursor *string     `json:"next_cursor"`
}
type sortKey struct {
	Name string `json:"name"`
	Desc bool   `json:"desc"`
}
type listQuery struct {
	KindID      *string   `json:"kind_id"`
	Kinds       []string  `json:"kinds"`
	States      []string  `json:"states"`
	Priorities  []string  `json:"priorities"`
	Assignees   []string  `json:"assignees"`
	Q           string    `json:"q"`
	Within      *string   `json:"within"`
	ParentSet   bool      `json:"parent_set"`
	ParentID    *string   `json:"parent_id"`
	Descendants bool      `json:"descendants"`
	HideClosed  bool      `json:"hide_closed"`
	Sort        []sortKey `json:"sort"`
	FacetNames  []string  `json:"facets"`
	Limit       int       `json:"limit"`
	Cursor      string    `json:"-"`
}
type listCursor struct {
	Hash string `json:"hash"`
	ID   string `json:"id"`
}
type treeQuery struct {
	RootID   *string
	DepthSet bool
	MaxDepth int
	Limit    int
	Cursor   string
}

var validSort = map[string]bool{"key": true, "title": true, "state": true, "priority": true, "kind": true, "updated_at": true, "created_at": true, "position": true}
var validFacet = map[string]bool{"state": true, "kind": true, "priority": true, "assignee": true}
var slugOrID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

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
func commaValues(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			v := strings.TrimSpace(part)
			if !seen[v] {
				out = append(out, v)
				seen[v] = true
			}
		}
	}
	return out
}
func queryList(r *http.Request, name string) ([]string, error) {
	q := r.URL.Query()
	if !q.Has(name) {
		return nil, nil
	}
	values := commaValues(q[name])
	for _, v := range values {
		if v == "" {
			return nil, badRequest("invalid " + name)
		}
	}
	return values, nil
}
func queryBool(r *http.Request, name string) (bool, error) {
	if !r.URL.Query().Has(name) {
		return false, nil
	}
	switch r.URL.Query().Get(name) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, badRequest("invalid " + name)
	}
}
func parseListQuery(r *http.Request) (listQuery, error) {
	v := r.URL.Query()
	out := listQuery{Limit: 50}
	if v.Has("kind_id") {
		id, ok := parseUUID(v.Get("kind_id"))
		if !ok {
			return out, badRequest("invalid kind_id")
		}
		out.KindID = &id
	}
	var err error
	if out.Kinds, err = queryList(r, "kind"); err != nil {
		return out, err
	}
	for _, kind := range out.Kinds {
		if _, ok := parseUUID(kind); !ok && !slugOrID.MatchString(kind) {
			return out, badRequest("invalid kind")
		}
	}
	if out.States, err = queryList(r, "state"); err != nil {
		return out, err
	}
	if out.Priorities, err = queryList(r, "priority"); err != nil {
		return out, err
	}
	if out.Assignees, err = queryList(r, "assignee"); err != nil {
		return out, err
	}
	for _, a := range out.Assignees {
		if a != "none" {
			if _, ok := parseUUID(a); !ok {
				return out, badRequest("invalid assignee")
			}
		}
	}
	if out.FacetNames, err = queryList(r, "facets"); err != nil {
		return out, err
	}
	for _, f := range out.FacetNames {
		if !validFacet[f] {
			return out, badRequest("invalid facets")
		}
	}
	out.Q = strings.TrimSpace(v.Get("q"))
	if v.Has("within") {
		id, ok := parseUUID(v.Get("within"))
		if !ok {
			return out, badRequest("invalid within")
		}
		out.Within = &id
	}
	if v.Has("parent_id") {
		id, ok := parseUUID(v.Get("parent_id"))
		if !ok {
			return out, badRequest("invalid parent_id")
		}
		out.ParentSet = true
		out.ParentID = &id
	}
	if out.Descendants, err = queryBool(r, "include_descendants"); err != nil {
		return out, err
	}
	if out.HideClosed, err = queryBool(r, "hide_closed"); err != nil {
		return out, err
	}
	if out.Descendants && !out.ParentSet && out.Within == nil {
		return out, badRequest("include_descendants requires parent_id")
	}
	if out.Within != nil && (out.ParentSet || v.Has("include_descendants")) {
		return out, badRequest("within conflicts with parent_id and include_descendants")
	}
	direction := "asc"
	if v.Has("direction") {
		direction = v.Get("direction")
		if direction != "asc" && direction != "desc" {
			return out, badRequest("invalid direction")
		}
	}
	sortRaw := "position"
	if v.Has("sort") {
		sortRaw = v.Get("sort")
	}
	for _, s := range strings.Split(sortRaw, ",") {
		s = strings.TrimSpace(s)
		desc := direction == "desc"
		if strings.HasPrefix(s, "-") {
			s = strings.TrimPrefix(s, "-")
			desc = true
		}
		if !validSort[s] {
			return out, badRequest("invalid sort")
		}
		out.Sort = append(out.Sort, sortKey{s, desc})
	}
	out.Limit, err = parseLimit(v.Get("limit"), v.Has("limit"))
	if err != nil {
		return out, err
	}
	out.Cursor = v.Get("cursor")
	return out, nil
}
func listFingerprint(q listQuery) string {
	q.Cursor = ""
	raw, _ := json.Marshal(q)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (m *Module) listNodes(ctx context.Context, tenantID string, q listQuery) (nodePage, error) {
	var mark *listCursor
	if q.Cursor != "" {
		env, err := openCursor(q.Cursor)
		if err != nil {
			return nodePage{}, err
		}
		if env.T != "list" {
			return nodePage{}, badRequest("cursor does not match this query")
		}
		var c listCursor
		if json.Unmarshal(env.P, &c) != nil || c.Hash != listFingerprint(q) {
			return nodePage{}, badRequest("cursor does not match this query")
		}
		if _, ok := parseUUID(c.ID); !ok {
			return nodePage{}, badRequest("invalid cursor")
		}
		mark = &c
	}
	page := nodePage{Items: []listItem{}}
	err := m.tx(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var anchor any
		if mark != nil {
			anchor = mark.ID
		}
		sql, args := listSQL(q, anchor)
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return dbErr("list nodes", err)
		}
		for rows.Next() {
			var item listItem
			var fields, position string
			var assigneeID, assigneeName, parentID, parentKey, parentTitle, parentKind, projectID, projectKey, projectTitle, epicID, epicKey, epicTitle *string
			err = rows.Scan(&item.ID, &item.Key, &item.KindID, &item.Title, &item.Body, &fields, &item.State, &item.ParentID, &position, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.KindSlug, &item.KindLabel, &item.Priority, &assigneeID, &assigneeName, &parentID, &parentKey, &parentTitle, &parentKind, &item.ChildrenCount, &projectID, &projectKey, &projectTitle, &epicID, &epicKey, &epicTitle)
			if err != nil {
				rows.Close()
				return err
			}
			item.Fields = json.RawMessage(fields)
			item.Position = trimDecimal(position)
			if assigneeID != nil {
				item.Assignee = &listPerson{*assigneeID, *assigneeName}
			}
			if parentID != nil {
				item.Parent = &listParent{*parentID, *parentKey, *parentTitle, *parentKind}
			}
			if projectID != nil {
				item.Project = &listProject{*projectID, *projectKey, *projectTitle}
			}
			if epicID != nil {
				item.Epic = &listEpic{*epicID, *epicKey, *epicTitle}
			}
			page.Items = append(page.Items, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(page.Items) > q.Limit {
			last := page.Items[q.Limit-1]
			page.Items = page.Items[:q.Limit]
			cursor, err := encodeTyped("list", listCursor{listFingerprint(q), last.ID})
			if err != nil {
				return err
			}
			page.NextCursor = &cursor
		}
		if len(q.FacetNames) > 0 {
			page.Facets = map[string]map[string]int{}
			for _, f := range q.FacetNames {
				page.Facets[f] = map[string]int{}
			}
			sql, args = facetSQL(q)
			rows, err = tx.Query(ctx, sql, args...)
			if err != nil {
				return dbErr("list facets", err)
			}
			for rows.Next() {
				var name, value string
				var count int
				if err = rows.Scan(&name, &value, &count); err != nil {
					rows.Close()
					return err
				}
				page.Facets[name][value] = count
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	return page, err
}

// Native fields win, including an explicit null (unassignment). Classic IDs
// are resolved only through this tenant's source-qualified identity mapping.
const assigneeJoin = ` LEFT JOIN LATERAL (
    SELECT coalesce(target.id,person.id) AS id,coalesce(target.name,person.name) AS name FROM principals person
    LEFT JOIN principals target ON target.tenant_id=person.tenant_id AND target.id=person.linked_to
    LEFT JOIN identities identity ON identity.id=person.identity_id
    WHERE person.tenant_id=n.tenant_id AND (
        person.id::text = CASE
            WHEN n.fields ? 'assignee' THEN coalesce(n.fields->'assignee'->>'id',n.fields->>'assignee')
            WHEN n.fields ? 'assignee_id' THEN n.fields->>'assignee_id' END
        OR (NOT (n.fields ? 'assignee' OR n.fields ? 'assignee_id')
            AND identity.issuer='paimos-classic'
            AND identity.subject=(n.fields->'classic'->>'source_id')||':'||(n.fields->'classic'->>'assignee_id'))
    ) LIMIT 1
) assignee ON true `

func listFilterSQL(q listQuery) (string, []any) {
	// One tenant transaction supplies RLS to every table in the CTE.
	array := func(values []string) []string {
		if values == nil {
			return []string{}
		}
		return values
	}
	args := []any{q.KindID, array(q.Kinds), array(q.States), array(q.Priorities), array(q.Assignees), q.Q, q.Within, q.ParentSet, q.ParentID, q.Descendants, q.HideClosed}
	return `WITH RECURSIVE scope(id) AS (
        SELECT id FROM nodes WHERE tenant_id=current_setting('aeon.tenant_id')::uuid AND deleted_at IS NULL AND id=coalesce($7::uuid, CASE WHEN $8::bool AND $10::bool THEN $9::uuid ELSE NULL::uuid END)
        UNION ALL SELECT c.id FROM scope s CROSS JOIN LATERAL (
            SELECT id FROM nodes WHERE tenant_id=current_setting('aeon.tenant_id')::uuid AND parent_id=s.id AND deleted_at IS NULL OFFSET 0
        ) c
    ), filtered AS (
        SELECT n.id,assignee.id::text AS assignee_id FROM nodes n JOIN node_kinds k ON k.id=n.kind_id AND k.tenant_id=n.tenant_id ` + assigneeJoin + `
        WHERE n.deleted_at IS NULL
        AND ($1::uuid IS NULL OR n.kind_id=$1::uuid)
        AND (cardinality($2::text[])=0 OR k.slug=ANY($2::text[]) OR n.kind_id::text=ANY($2::text[]))
        AND (cardinality($3::text[])=0 OR n.state=ANY($3::text[]))
        AND (cardinality($4::text[])=0 OR coalesce(nullif(n.fields->>'priority',''),'none')=ANY($4::text[]))
        AND (cardinality($5::text[])=0 OR coalesce(assignee.id::text,'none')=ANY($5::text[])
            OR assignee.id IN (SELECT coalesce(linked_to,id) FROM principals WHERE id::text=ANY($5::text[])))
        AND ($6::text='' OR n.key ILIKE '%'||$6::text||'%' OR n.title ILIKE '%'||$6::text||'%'
            OR EXISTS(SELECT 1 FROM node_key_aliases a WHERE a.tenant_id=n.tenant_id AND a.node_id=n.id
                AND a.key ILIKE '%'||$6::text||'%'))
        AND ($7::uuid IS NULL OR (n.id IN (SELECT id FROM scope) AND n.id<>$7::uuid))
        AND ($7::uuid IS NOT NULL OR NOT $8::bool OR (CASE WHEN $10::bool THEN n.id IN (SELECT id FROM scope) AND n.id<>$9::uuid ELSE n.parent_id IS NOT DISTINCT FROM $9::uuid END))
        AND (NOT $11::bool OR n.state NOT IN ('done','cancelled','archived','delivered','accepted'))
    )`, args
}
func listOrder(q listQuery) string {
	parts := []string{}
	if q.Q != "" {
		parts = append(parts, "CASE WHEN n.key ILIKE $6::text||'%' THEN 0 ELSE 1 END ASC")
	}
	for _, key := range q.Sort {
		dir := "ASC"
		if key.Desc {
			dir = "DESC"
		}
		switch key.Name {
		case "key":
			parts = append(parts, `regexp_replace(n.key,'-[0-9]+$','') `+dir, `substring(n.key from '-([0-9]+)$')::numeric `+dir)
		case "state":
			parts = append(parts, `CASE WHEN n.state IN ('new','backlog','in_progress','active','qa','accepted','delivered','done','cancelled','archived') THEN 0 ELSE 1 END ASC`, `CASE n.state WHEN 'new' THEN 0 WHEN 'backlog' THEN 1 WHEN 'in_progress' THEN 2 WHEN 'active' THEN 2 WHEN 'qa' THEN 3 WHEN 'accepted' THEN 4 WHEN 'delivered' THEN 5 WHEN 'done' THEN 6 WHEN 'cancelled' THEN 7 WHEN 'archived' THEN 8 ELSE 9 END `+dir, "n.state "+dir)
		case "priority":
			parts = append(parts, `CASE coalesce(nullif(n.fields->>'priority',''),'none') WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 WHEN 'none' THEN 3 ELSE 4 END `+dir, `coalesce(n.fields->>'priority','') `+dir)
		case "kind":
			parts = append(parts, "k.slug "+dir)
		default:
			parts = append(parts, "n."+key.Name+" "+dir)
		}
	}
	return strings.Join(append(parts, "n.id ASC"), ",")
}
func listSQL(q listQuery, anchor any) (string, []any) {
	prefix, args := listFilterSQL(q)
	args = append(args, anchor, q.Limit+1)
	sql := prefix + `, ordered AS (SELECT n.id,row_number() OVER (ORDER BY ` + listOrder(q) + `) AS rn FROM filtered f JOIN nodes n ON n.id=f.id JOIN node_kinds k ON k.id=n.kind_id),
    selected AS (SELECT id,rn FROM ordered WHERE rn>coalesce((SELECT rn FROM ordered WHERE id=$12::uuid),0) ORDER BY rn LIMIT $13)
    SELECT ` + nodeCols + `,k.slug,k.label,nullif(n.fields->>'priority',''),assignee.id::text,assignee.name,
           par.id::text,par.key,par.title,pk.slug,
           (SELECT count(*)::int FROM nodes c WHERE c.parent_id=n.id AND c.deleted_at IS NULL),
           project.id::text,project.key,project.title,
           epic.id::text,epic.key,epic.title
    FROM selected s JOIN nodes n ON n.id=s.id JOIN node_kinds k ON k.id=n.kind_id
    ` + assigneeJoin + `
    LEFT JOIN nodes par ON par.id=n.parent_id AND par.deleted_at IS NULL
    LEFT JOIN node_kinds pk ON pk.id=par.kind_id
    LEFT JOIN LATERAL (
        WITH RECURSIVE ancestors AS (
            SELECT n.id,n.parent_id,n.kind_id,n.key,n.title,0 AS depth
            UNION ALL SELECT a.id,a.parent_id,a.kind_id,a.key,a.title,anc.depth+1 FROM nodes a JOIN ancestors anc ON a.id=anc.parent_id WHERE a.deleted_at IS NULL
        ) SELECT a.id,a.key,a.title FROM ancestors a JOIN node_kinds ak ON ak.id=a.kind_id WHERE ak.slug='project' ORDER BY a.depth LIMIT 1
    ) project ON true
    LEFT JOIN LATERAL (
        -- The nearest epic above the item; the walk stops at the first epic or project.
        WITH RECURSIVE up AS (
            SELECT par.id,par.parent_id,par.kind_id,par.key,par.title,1 AS depth WHERE par.id IS NOT NULL
            UNION ALL SELECT a.id,a.parent_id,a.kind_id,a.key,a.title,up.depth+1 FROM up
                JOIN node_kinds uk ON uk.id=up.kind_id AND uk.slug NOT IN ('epic','project')
                JOIN nodes a ON a.id=up.parent_id AND a.deleted_at IS NULL
            WHERE up.depth<32
        ) SELECT u.id,u.key,u.title FROM up u JOIN node_kinds ek ON ek.id=u.kind_id WHERE ek.slug='epic' ORDER BY u.depth LIMIT 1
    ) epic ON true
    ORDER BY s.rn`
	return sql, args
}
func facetSQL(q listQuery) (string, []any) {
	prefix, args := listFilterSQL(q)
	sql := prefix + `, facet_values AS (
    SELECT 'state' AS name,n.state AS value FROM filtered f JOIN nodes n ON n.id=f.id
    UNION ALL SELECT 'kind',k.slug FROM filtered f JOIN nodes n ON n.id=f.id JOIN node_kinds k ON k.id=n.kind_id
    UNION ALL SELECT 'priority',coalesce(nullif(n.fields->>'priority',''),'none') FROM filtered f JOIN nodes n ON n.id=f.id
    UNION ALL SELECT 'assignee',coalesce(f.assignee_id,'none') FROM filtered f
    ) SELECT name,value,count(*)::int FROM facet_values WHERE name=ANY($12::text[]) GROUP BY name,value`
	return sql, append(args, q.FacetNames)
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
	if err != nil || n < 1 || n > 500 {
		return 0, badRequest("invalid limit")
	}
	return n, nil
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

type projectSummary struct {
	ID           string    `json:"id"`
	Key          string    `json:"key"`
	Title        string    `json:"title"`
	State        string    `json:"state"`
	Open         int       `json:"open"`
	InProgress   int       `json:"in_progress"`
	Done         int       `json:"done"`
	Cancelled    int       `json:"cancelled"`
	Total        int       `json:"total"`
	LastActivity time.Time `json:"last_activity"`
	// The people (and agents) most recently active in the project, newest first:
	// actors of the tenant's latest events (90 days, at most 20000) on the project
	// or anything below it, a linked principal shown as the person it links to.
	People []projectPerson `json:"people"`
}
type projectPerson struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// recentPeoplePerProject bounds the avatars a project summary carries.
const recentPeoplePerProject = 5

type projectPage struct {
	Items []projectSummary `json:"items"`
}

func (m *Module) handleListProjects(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	includeArchived, err := queryBool(r, "include_archived")
	if err != nil {
		writeErr(w, err)
		return
	}
	page := projectPage{Items: []projectSummary{}}
	err = m.tx(r.Context(), p.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
            WITH RECURSIVE projects AS (
                SELECT n.id,n.key,n.title,n.state,n.updated_at
                FROM nodes n JOIN node_kinds k ON k.id=n.kind_id AND k.tenant_id=n.tenant_id
                WHERE n.tenant_id=current_setting('aeon.tenant_id')::uuid AND n.deleted_at IS NULL AND k.slug='project' AND ($1::bool OR n.state<>'archived')
            ), subtree AS (
                SELECT p.id AS project_id,p.id AS node_id,n.parent_id,n.state,n.updated_at,n.kind_id,0 AS depth
                FROM projects p JOIN nodes n ON n.id=p.id
                UNION ALL
                SELECT s.project_id,c.id,c.parent_id,c.state,c.updated_at,c.kind_id,s.depth+1
                FROM subtree s CROSS JOIN LATERAL (
                    SELECT id,parent_id,state,updated_at,kind_id FROM nodes
                    WHERE tenant_id=current_setting('aeon.tenant_id')::uuid AND parent_id=s.node_id AND deleted_at IS NULL OFFSET 0
                ) c
            ), summary AS (
                SELECT p.id,p.key,p.title,p.state,
                    count(*) FILTER (WHERE s.depth>0 AND k.slug IN ('ticket','task','epic') AND s.state IN ('new','backlog'))::int AS open,
                    count(*) FILTER (WHERE s.depth>0 AND k.slug IN ('ticket','task','epic') AND s.state IN ('in_progress','qa'))::int AS in_progress,
                    count(*) FILTER (WHERE s.depth>0 AND k.slug IN ('ticket','task','epic') AND s.state IN ('accepted','delivered','done'))::int AS done,
                    count(*) FILTER (WHERE s.depth>0 AND k.slug IN ('ticket','task','epic') AND s.state='cancelled')::int AS cancelled,
                    count(*) FILTER (WHERE s.depth>0 AND k.slug IN ('ticket','task','epic'))::int AS total,
                    max(s.updated_at) AS last_activity
                FROM projects p JOIN subtree s ON s.project_id=p.id
                JOIN node_kinds k ON k.id=s.kind_id AND k.tenant_id=current_setting('aeon.tenant_id')::uuid
                GROUP BY p.id,p.key,p.title,p.state
            ), recent AS (
                SELECT e.node_id,e.actor_principal_id,e.at FROM events e
                WHERE e.tenant_id=current_setting('aeon.tenant_id')::uuid AND e.node_id IS NOT NULL AND e.at > now() - interval '90 days'
                ORDER BY e.at DESC,e.id DESC LIMIT 20000
            ), actors AS (
                SELECT s.project_id,coalesce(a.linked_to,a.id) AS person,max(r.at) AS at
                FROM recent r JOIN subtree s ON s.node_id=r.node_id
                JOIN principals a ON a.tenant_id=current_setting('aeon.tenant_id')::uuid AND a.id=r.actor_principal_id
                GROUP BY s.project_id,coalesce(a.linked_to,a.id)
            ), ranked AS (
                SELECT project_id,person,at,row_number() OVER (PARTITION BY project_id ORDER BY at DESC,person) AS n FROM actors
            ), people AS (
                SELECT r.project_id,json_agg(json_build_object('id',who.id::text,'name',who.name,'kind',who.kind) ORDER BY r.at DESC,r.person) AS people
                FROM ranked r JOIN principals who ON who.tenant_id=current_setting('aeon.tenant_id')::uuid AND who.id=r.person
                WHERE r.n <= $2
                GROUP BY r.project_id
            )
            SELECT s.id::text,s.key,s.title,s.state,s.open,s.in_progress,s.done,s.cancelled,s.total,s.last_activity,coalesce(pp.people,'[]'::json)
            FROM summary s LEFT JOIN people pp ON pp.project_id=s.id
            ORDER BY s.last_activity DESC,s.id`, includeArchived, recentPeoplePerProject)
		if err != nil {
			return dbErr("list projects", err)
		}
		for rows.Next() {
			var item projectSummary
			var people []byte
			if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.State, &item.Open, &item.InProgress, &item.Done, &item.Cancelled, &item.Total, &item.LastActivity, &people); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal(people, &item.People); err != nil || item.People == nil {
				item.People = []projectPerson{}
			}
			page.Items = append(page.Items, item)
		}
		err = rows.Err()
		rows.Close()
		return err
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
