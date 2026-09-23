// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// issueView is the classic issue text/JSON shape. Aeon stores the issue as a
// node: type is the kind slug, status is state, and priority lives in fields.
type issueView struct {
	IssueKey    string   `json:"issue_key"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	Description string   `json:"description,omitempty"`
	ID          string   `json:"id"`
	Assignee    string   `json:"assignee,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Comments    []string `json:"comments,omitempty"`
}

type issueInput struct {
	Project     string
	Title       string
	Type        string
	Status      string
	Priority    string
	Parent      string
	Assignee    string
	Description string
	AC          string
	Notes       string
	Tags        []string
}

var issueKinds = map[string]bool{"epic": true, "ticket": true, "task": true}

func (rt *runtime) viewIssue(n apiNode, kinds kindTable) issueView {
	fields := fieldMap(n.Fields)
	var comments []string
	if raw, ok := fields["comments"].([]any); ok {
		for _, item := range raw {
			body, _ := item.(map[string]any)
			if text := fieldString(body, "body"); text != "" {
				comments = append(comments, text)
			}
		}
	}
	return issueView{
		IssueKey:    n.Key,
		Title:       n.Title,
		Type:        kinds.slug(n.KindID),
		Status:      n.State,
		Priority:    fieldString(fields, "priority"),
		Description: n.Body,
		ID:          n.ID,
		Assignee:    fieldString(fields, "assignee"),
		Tags:        fieldStrings(fields, "tags"),
		Comments:    comments,
	}
}

func (rt *runtime) printIssue(v issueView) error {
	if rt.jsonOut {
		return rt.printJSON(v)
	}
	fmt.Fprintf(rt.stdout, "%s  %s\n", v.IssueKey, v.Title)
	fmt.Fprintf(rt.stdout, "  type:     %s\n", v.Type)
	fmt.Fprintf(rt.stdout, "  status:   %s\n", v.Status)
	fmt.Fprintf(rt.stdout, "  priority: %s\n", v.Priority)
	if v.Description != "" {
		desc := clipRunes(v.Description, 160, "…")
		fmt.Fprintf(rt.stdout, "\n  %s\n", strings.ReplaceAll(desc, "\n", "\n  "))
	}
	return nil
}

func (rt *runtime) printIssueList(items []issueView, total int) error {
	if rt.jsonOut {
		return rt.printJSON(map[string]any{"issues": items, "total": total})
	}
	if len(items) == 0 {
		fmt.Fprintln(rt.stdout, "(no issues)")
		return nil
	}
	fmt.Fprintln(rt.stdout, "KEY           STATUS         PRIO   TITLE")
	for _, item := range items {
		fmt.Fprintf(rt.stdout, "%-13s %-14s %-6s %s\n", item.IssueKey, item.Status, item.Priority, clipRunes(item.Title, 60, "…"))
	}
	if total > len(items) {
		fmt.Fprintf(rt.stdout, "\n(showing %d of %d — use --limit / --offset for more)\n", len(items), total)
	}
	return nil
}

func (rt *runtime) listIssues(project, status, typ, priority, assignee string, limit, offset int) error {
	if typ != "" && !issueKinds[typ] {
		return usagef("unknown issue type %q", typ)
	}
	proj, err := rt.projectNode(project)
	if err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	q := url.Values{"parent_id": {proj.ID}, "include_descendants": {"true"}}
	if status != "" {
		q.Set("state", status)
	}
	if typ != "" {
		k, ok := kinds.bySlug[typ]
		if !ok {
			return rt.fail(fmt.Errorf("node kind %q is not configured", typ), "")
		}
		q.Set("kind_id", k.ID)
	}
	nodes, err := rt.walkNodes(q, nil)
	if err != nil {
		return err
	}
	var matched []issueView
	for _, n := range nodes {
		slug := kinds.slug(n.KindID)
		if typ == "" && !issueKinds[slug] {
			continue
		}
		fields := fieldMap(n.Fields)
		if priority != "" && fieldString(fields, "priority") != priority {
			continue
		}
		if assignee != "" && fieldString(fields, "assignee") != assignee {
			continue
		}
		matched = append(matched, rt.viewIssue(n, kinds))
	}
	if limit <= 0 {
		limit = 50
	}
	if offset > len(matched) {
		offset = len(matched)
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return rt.printIssueList(matched[offset:end], len(matched))
}

func (rt *runtime) getIssue(ref string) error {
	if strings.HasPrefix(strings.TrimSpace(ref), "id:") {
		return usagef("id:<n> is a classic numeric id; pass the issue key")
	}
	n, err := rt.nodeByKey(ref)
	if err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	if !issueKinds[kinds.slug(n.KindID)] {
		return rt.fail(fmt.Errorf("issue %q not found", ref), "")
	}
	return rt.printIssue(rt.viewIssue(n, kinds))
}

func (rt *runtime) createIssue(in issueInput) error {
	kindName := strings.TrimSpace(in.Type)
	if kindName == "" {
		kindName = "ticket"
	}
	if !issueKinds[kindName] {
		return usagef("unknown issue type %q", kindName)
	}
	prefix := strings.TrimSpace(in.Project)
	if !validKeyPrefix(prefix) {
		return usagef("project key %q cannot allocate issue keys", prefix)
	}
	proj, err := rt.projectNode(prefix)
	if err != nil {
		return err
	}
	kind, err := rt.kind(kindName)
	if err != nil {
		return err
	}
	parentID := proj.ID
	if strings.TrimSpace(in.Parent) != "" {
		if strings.HasPrefix(strings.TrimSpace(in.Parent), "id:") {
			return usagef("id:<n> is a classic numeric id; pass the issue key")
		}
		parentRef, err := normalizeIssueRef(in.Parent)
		if err != nil {
			return err
		}
		parent, err := rt.nodeByKey(parentRef)
		if err != nil {
			return err
		}
		parentID = parent.ID
	}
	fields := map[string]any{}
	if p := strings.TrimSpace(in.Priority); p != "" {
		fields["priority"] = p
	}
	if a := strings.TrimSpace(in.Assignee); a != "" {
		fields["assignee"] = a
	}
	if in.AC != "" {
		fields["acceptance_criteria"] = in.AC
	}
	if in.Notes != "" {
		fields["notes"] = in.Notes
	}
	if len(in.Tags) > 0 {
		fields["tags"] = in.Tags
	}
	body := map[string]any{
		"kind_id":    kind.ID,
		"title":      strings.TrimSpace(in.Title),
		"body":       in.Description,
		"parent_id":  parentID,
		"key_prefix": prefix,
		"fields":     fields,
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		body["state"] = status
	}
	var created apiNode
	if err := rt.do(http.MethodPost, "/api/nodes", body, &created); err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	view := rt.viewIssue(created, kinds)
	if rt.jsonOut {
		return rt.printJSON(view)
	}
	fmt.Fprintf(rt.stdout, "✓ created %s — %s\n", view.IssueKey, view.Title)
	return nil
}

type issuePatch struct {
	Ref         string
	Title       string
	Type        string
	Status      string
	Priority    string
	Parent      string
	Assignee    string
	Project     string
	Description string
	AC          string
	Notes       string
	CloseNote   string
	AddTag      []string
	RemoveTag   []string
}

func (rt *runtime) updateIssue(in issuePatch) error {
	if strings.HasPrefix(strings.TrimSpace(in.Ref), "id:") {
		return usagef("id:<n> is a classic numeric id; pass the issue key")
	}
	if strings.TrimSpace(in.Type) != "" && !issueKinds[in.Type] {
		return usagef("unknown issue type %q (kind is immutable; create a node of that kind)", in.Type)
	}
	n, err := rt.nodeByKey(in.Ref)
	if err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	if !issueKinds[kinds.slug(n.KindID)] {
		return rt.fail(fmt.Errorf("issue %q not found", in.Ref), "")
	}
	fields := fieldMap(n.Fields)
	changedFields := false
	if p := strings.TrimSpace(in.Priority); p != "" {
		fields["priority"] = p
		changedFields = true
	}
	if a := strings.TrimSpace(in.Assignee); a != "" {
		fields["assignee"] = a
		changedFields = true
	}
	if in.AC != "" {
		fields["acceptance_criteria"] = in.AC
		changedFields = true
	}
	if in.Notes != "" {
		fields["notes"] = in.Notes
		changedFields = true
	}
	if in.CloseNote != "" {
		fields["close_note"] = in.CloseNote
		changedFields = true
	}
	if len(in.AddTag) > 0 || len(in.RemoveTag) > 0 {
		tags := fieldStrings(fields, "tags")
		drop := map[string]bool{}
		for _, tag := range in.RemoveTag {
			drop[tag] = true
		}
		var next []string
		seen := map[string]bool{}
		for _, tag := range tags {
			if drop[tag] || seen[tag] {
				continue
			}
			seen[tag] = true
			next = append(next, tag)
		}
		for _, tag := range in.AddTag {
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			next = append(next, tag)
		}
		fields["tags"] = next
		changedFields = true
	}
	patch := map[string]any{}
	if title := strings.TrimSpace(in.Title); title != "" {
		patch["title"] = title
	}
	if in.Description != "" {
		patch["body"] = in.Description
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		patch["state"] = status
	}
	if changedFields {
		patch["fields"] = fields
	}
	oldStatus := n.State
	if len(patch) > 0 {
		if err := rt.do(http.MethodPatch, "/api/nodes/"+url.PathEscape(n.ID), patch, &n); err != nil {
			return err
		}
	}
	if project := strings.TrimSpace(in.Project); project != "" {
		proj, err := rt.projectNode(project)
		if err != nil {
			return err
		}
		if n.ParentID == nil || *n.ParentID != proj.ID {
			if err := rt.do(http.MethodPost, "/api/nodes/"+url.PathEscape(n.ID)+"/move", map[string]any{"parent_id": proj.ID}, &n); err != nil {
				return err
			}
		}
	}
	if parent := strings.TrimSpace(in.Parent); parent != "" {
		if strings.HasPrefix(parent, "id:") {
			return usagef("id:<n> is a classic numeric id; pass the issue key")
		}
		ref, err := normalizeIssueRef(parent)
		if err != nil {
			return err
		}
		target, err := rt.nodeByKey(ref)
		if err != nil {
			return err
		}
		if n.ParentID == nil || *n.ParentID != target.ID {
			if err := rt.do(http.MethodPost, "/api/nodes/"+url.PathEscape(n.ID)+"/move", map[string]any{"parent_id": target.ID}, &n); err != nil {
				return err
			}
		}
	}
	view := rt.viewIssue(n, kinds)
	if rt.jsonOut {
		return rt.printJSON(view)
	}
	if strings.TrimSpace(in.Status) != "" && strings.TrimSpace(in.Title+in.Description+in.Priority+in.Assignee+in.Project+in.Parent+in.AC+in.Notes+in.CloseNote) == "" && len(in.AddTag) == 0 && len(in.RemoveTag) == 0 {
		fmt.Fprintf(rt.stdout, "✓ %s: %s → %s\n", view.IssueKey, oldStatus, view.Status)
		return nil
	}
	fmt.Fprintf(rt.stdout, "✓ updated %s\n", view.IssueKey)
	return nil
}

func (rt *runtime) commentIssue(ref, body string) error {
	if strings.HasPrefix(strings.TrimSpace(ref), "id:") {
		return usagef("id:<n> is a classic numeric id; pass the issue key")
	}
	n, err := rt.nodeByKey(ref)
	if err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	if !issueKinds[kinds.slug(n.KindID)] {
		return rt.fail(fmt.Errorf("issue %q not found", ref), "")
	}
	fields := fieldMap(n.Fields)
	comments, _ := fields["comments"].([]any)
	fields["comments"] = append(comments, map[string]any{"body": body})
	if err := rt.do(http.MethodPatch, "/api/nodes/"+url.PathEscape(n.ID), map[string]any{"fields": fields}, &n); err != nil {
		return err
	}
	if rt.jsonOut {
		return rt.printJSON(rt.viewIssue(n, kinds))
	}
	fmt.Fprintf(rt.stdout, "✓ commented on %s\n", n.Key)
	return nil
}

func validKeyPrefix(s string) bool {
	if len(s) < 2 || len(s) > 10 {
		return false
	}
	for i, r := range s {
		if i == 0 && (r < 'A' || r > 'Z') {
			return false
		}
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

type knowledgeView struct {
	Type   string `json:"type"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Body   string `json:"body,omitempty"`
	Key    string `json:"key"`
	ID     string `json:"id"`
}

func knowledgeSupported(typ string) bool {
	switch typ {
	case "memory", "runbook", "guideline":
		return true
	default:
		return false
	}
}

func (rt *runtime) rejectKnowledgeKind(typ string) error {
	if knowledgeSupported(typ) {
		return nil
	}
	return notYet(reasonKnowledgeKind)
}

func viewKnowledge(n apiNode, kinds kindTable) knowledgeView {
	fields := fieldMap(n.Fields)
	return knowledgeView{
		Type:   kinds.slug(n.KindID),
		Slug:   fieldString(fields, "slug"),
		Title:  n.Title,
		Status: n.State,
		Body:   n.Body,
		Key:    n.Key,
		ID:     n.ID,
	}
}

func (rt *runtime) printKnowledge(v knowledgeView) error {
	if rt.jsonOut {
		return rt.printJSON(v)
	}
	fmt.Fprintf(rt.stdout, "%s/%s (%s)\n", v.Type, v.Slug, v.Key)
	fmt.Fprintf(rt.stdout, "  title:  %s\n", v.Title)
	fmt.Fprintf(rt.stdout, "  status: %s\n", v.Status)
	if strings.TrimSpace(v.Body) != "" {
		fmt.Fprintln(rt.stdout, "  body:")
		for _, line := range strings.Split(v.Body, "\n") {
			fmt.Fprintln(rt.stdout, "    "+line)
		}
	}
	return nil
}

func (rt *runtime) knowledgeNodes(project, typ string) (kindTable, []apiNode, error) {
	if err := rt.rejectKnowledgeKind(typ); err != nil && typ != "" {
		return kindTable{}, nil, err
	}
	proj, err := rt.projectNode(project)
	if err != nil {
		return kindTable{}, nil, err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return kindTable{}, nil, err
	}
	q := url.Values{"parent_id": {proj.ID}, "include_descendants": {"true"}}
	if typ != "" {
		k, ok := kinds.bySlug[typ]
		if !ok {
			return kindTable{}, nil, rt.fail(fmt.Errorf("node kind %q is not configured", typ), "")
		}
		q.Set("kind_id", k.ID)
	}
	nodes, err := rt.walkNodes(q, nil)
	if err != nil {
		return kindTable{}, nil, err
	}
	var kept []apiNode
	for _, n := range nodes {
		slug := kinds.slug(n.KindID)
		if !knowledgeSupported(slug) {
			continue
		}
		if typ != "" && slug != typ {
			continue
		}
		kept = append(kept, n)
	}
	return kinds, kept, nil
}

func (rt *runtime) listKnowledge(project, typ string) error {
	kinds, nodes, err := rt.knowledgeNodes(project, typ)
	if err != nil {
		return err
	}
	var items []knowledgeView
	for _, n := range nodes {
		items = append(items, viewKnowledge(n, kinds))
	}
	if rt.jsonOut {
		return rt.printJSON(map[string]any{"entries": items})
	}
	if len(items) == 0 {
		fmt.Fprintln(rt.stdout, "(no entries)")
		return nil
	}
	fmt.Fprintln(rt.stdout, "TYPE             SLUG                            STATUS       TITLE")
	for _, e := range items {
		fmt.Fprintf(rt.stdout, "%-16s %-31s %-12s %s\n", e.Type, e.Slug, e.Status, clipRunes(e.Title, 60, "..."))
	}
	return nil
}

func (rt *runtime) getKnowledge(typ, slug, project string) error {
	kinds, nodes, err := rt.knowledgeNodes(project, typ)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if fieldString(fieldMap(n.Fields), "slug") == slug {
			return rt.printKnowledge(viewKnowledge(n, kinds))
		}
	}
	return rt.fail(fmt.Errorf("knowledge %s/%s not found", typ, slug), "")
}

func (rt *runtime) createKnowledge(project, typ, slug, title, body, status string) error {
	if err := rt.rejectKnowledgeKind(typ); err != nil {
		return err
	}
	proj, err := rt.projectNode(project)
	if err != nil {
		return err
	}
	kind, err := rt.kind(typ)
	if err != nil {
		return err
	}
	req := map[string]any{
		"kind_id":   kind.ID,
		"title":     strings.TrimSpace(title),
		"body":      body,
		"parent_id": proj.ID,
		"fields":    map[string]any{"slug": strings.TrimSpace(slug)},
	}
	if s := strings.TrimSpace(status); s != "" {
		req["state"] = s
	}
	var created apiNode
	if err := rt.do(http.MethodPost, "/api/nodes", req, &created); err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	view := viewKnowledge(created, kinds)
	if rt.jsonOut {
		return rt.printJSON(view)
	}
	fmt.Fprintf(rt.stdout, "✓ created %s/%s (%s)\n", view.Type, view.Slug, view.Key)
	return nil
}

func (rt *runtime) updateKnowledge(typ, slug, project, title, body, status, newSlug, metadata string) error {
	kinds, nodes, err := rt.knowledgeNodes(project, typ)
	if err != nil {
		return err
	}
	var n apiNode
	found := false
	for _, item := range nodes {
		if fieldString(fieldMap(item.Fields), "slug") == slug {
			n = item
			found = true
			break
		}
	}
	if !found {
		return rt.fail(fmt.Errorf("knowledge %s/%s not found", typ, slug), "")
	}
	fields := fieldMap(n.Fields)
	patch := map[string]any{}
	if t := strings.TrimSpace(title); t != "" {
		patch["title"] = t
	}
	if body != "" {
		patch["body"] = body
	}
	if s := strings.TrimSpace(status); s != "" {
		patch["state"] = s
	}
	if s := strings.TrimSpace(newSlug); s != "" {
		fields["slug"] = s
		patch["fields"] = fields
	}
	if metadata != "" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metadata), &meta); err != nil || meta == nil {
			return usagef("--metadata must be a JSON object")
		}
		fields["metadata"] = meta
		patch["fields"] = fields
	}
	if len(patch) == 0 {
		return usagef("nothing to update")
	}
	if err := rt.do(http.MethodPatch, "/api/nodes/"+url.PathEscape(n.ID), patch, &n); err != nil {
		return err
	}
	view := viewKnowledge(n, kinds)
	if rt.jsonOut {
		return rt.printJSON(view)
	}
	fmt.Fprintf(rt.stdout, "✓ updated %s/%s (%s)\n", view.Type, view.Slug, view.Key)
	return nil
}

func (rt *runtime) searchIssues(query, project, typ string, limit int) error {
	if typ != "" && !issueKinds[typ] {
		return usagef("unknown issue type %q", typ)
	}
	var parent string
	if strings.TrimSpace(project) != "" {
		proj, err := rt.projectNode(project)
		if err != nil {
			return err
		}
		parent = proj.ID
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	q := url.Values{"q": {query}}
	if typ != "" {
		k, ok := kinds.bySlug[typ]
		if !ok {
			return usagef("unknown issue type %q", typ)
		}
		q.Set("kind_id", k.ID)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var page struct {
		Items []struct {
			Node apiNode `json:"node"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := rt.do(http.MethodGet, "/api/search?"+q.Encode(), nil, &page); err != nil {
		return err
	}
	var items []issueView
	for _, hit := range page.Items {
		slug := kinds.slug(hit.Node.KindID)
		if !issueKinds[slug] {
			continue
		}
		if parent != "" && (hit.Node.ParentID == nil || *hit.Node.ParentID != parent) {
			continue
		}
		items = append(items, rt.viewIssue(hit.Node, kinds))
	}
	if rt.jsonOut {
		return rt.printJSON(map[string]any{"issues": items, "has_more": page.NextCursor != nil && *page.NextCursor != ""})
	}
	if len(items) == 0 {
		fmt.Fprintln(rt.stdout, "(no issues)")
		return nil
	}
	fmt.Fprintln(rt.stdout, "KEY           TYPE     STATUS         TITLE")
	for _, item := range items {
		fmt.Fprintf(rt.stdout, "%-13s %-8s %-14s %s\n", item.IssueKey, item.Type, item.Status, clipRunes(item.Title, 57, "..."))
	}
	if page.NextCursor != nil && *page.NextCursor != "" {
		fmt.Fprintln(rt.stdout, "\n(more issues available; raise --limit or use --json for has_more)")
	}
	return nil
}

func (rt *runtime) onboard(project, agent, format string) error {
	format = strings.TrimSpace(strings.ToLower(format))
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "html" {
		return usagef("--format must be md or html")
	}
	proj, err := rt.projectNode(project)
	if err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	children, err := rt.walkNodes(url.Values{"parent_id": {proj.ID}, "include_descendants": {"true"}}, nil)
	if err != nil {
		return err
	}
	var issues, knowledge []apiNode
	for _, n := range children {
		slug := kinds.slug(n.KindID)
		switch {
		case issueKinds[slug]:
			issues = append(issues, n)
		case knowledgeSupported(slug):
			knowledge = append(knowledge, n)
		}
	}
	ref := strings.TrimSpace(project)
	if stored := fieldString(fieldMap(proj.Fields), "project_key"); stored != "" {
		ref = stored
	}
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "agent"
	}
	if format == "html" {
		fmt.Fprintf(rt.stdout, "<h1>Welcome to %s</h1>\n", htmlEscape(proj.Title))
		if line := firstLine(proj.Body); line != "" {
			fmt.Fprintf(rt.stdout, "<blockquote>%s</blockquote>\n", htmlEscape(line))
		}
		fmt.Fprintf(rt.stdout, "<p>CLI quickstart: %s session start --project %s --agent %s</p>\n", rt.program, htmlEscape(ref), htmlEscape(agent))
		return nil
	}
	fmt.Fprintf(rt.stdout, "# Welcome to %s\n\n", proj.Title)
	if line := firstLine(proj.Body); line != "" {
		fmt.Fprintf(rt.stdout, "> %s\n\n", line)
	}
	if len(issues) > 0 {
		fmt.Fprintln(rt.stdout, "## Open work")
		fmt.Fprintln(rt.stdout)
		for _, n := range issues {
			fmt.Fprintf(rt.stdout, "- **%s** %s\n", n.Key, n.Title)
		}
		fmt.Fprintln(rt.stdout)
	}
	if len(knowledge) > 0 {
		fmt.Fprintln(rt.stdout, "## Knowledge")
		fmt.Fprintln(rt.stdout)
		for _, n := range knowledge {
			view := viewKnowledge(n, kinds)
			fmt.Fprintf(rt.stdout, "- **%s** (`%s/%s`)\n", view.Title, view.Type, view.Slug)
		}
		fmt.Fprintln(rt.stdout)
	}
	fmt.Fprintf(rt.stdout, "- CLI quickstart: `%s session start --project %s --agent %s`\n", rt.program, ref, agent)
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
