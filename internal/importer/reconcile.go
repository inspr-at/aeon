// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/inspr-at/aeon/internal/attachments"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileDifference names an imported classic record by its source ID.
// A source checksum includes classic content; attachment checksums are bytes.
type ReconcileDifference struct {
	ClassicID      string `json:"classic_id"`
	SourceChecksum string `json:"source_checksum,omitempty"`
	AeonChecksum   string `json:"aeon_checksum,omitempty"`
}

type ReconcileCategory struct {
	SourceCount    int                   `json:"source_count"`
	AeonCount      int                   `json:"aeon_count"`
	SourceChecksum string                `json:"source_checksum"`
	AeonChecksum   string                `json:"aeon_checksum"`
	Missing        []ReconcileDifference `json:"missing"`
	Extra          []ReconcileDifference `json:"extra"`
	Changed        []ReconcileDifference `json:"changed"`
}

type ReconcileProject struct {
	ClassicID  int64                        `json:"classic_id"`
	Key        string                       `json:"key"`
	Categories map[string]ReconcileCategory `json:"categories"`
}

type ReconcileReport struct {
	SourceID string             `json:"source_id"`
	Tenant   string             `json:"tenant"`
	Projects []ReconcileProject `json:"projects"`
	Summary  string             `json:"summary"`
}

type reconcileSet map[int64]map[string]map[string]string

func (s reconcileSet) put(project int64, category, id, checksum string) {
	if s[project] == nil {
		s[project] = map[string]map[string]string{}
	}
	if s[project][category] == nil {
		s[project][category] = map[string]string{}
	}
	s[project][category][id] = checksum
}

func checksum(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func recordChecksum(record Record, project bool, title, body, state string, fields Record) (string, error) {
	classic := Record{}
	for key, value := range record {
		if key == "source_id" || project && skippedProjectFields[key] {
			continue
		}
		classic[key] = value
	}
	return checksum(Record{"classic": classic, "title": title, "body": body, "state": canonicalState(state), "fields": fields})
}

func workFields(record Record, project bool) Record {
	fields := Record{}
	if project {
		if v, ok := record["tags"]; ok {
			fields["tags"] = v
		}
	} else {
		for _, name := range explicitIssueFields {
			if v, ok := record[name]; ok {
				fields[name] = v
			}
		}
	}
	return fields
}

func sourceNodeChecksum(record Record, project bool) (string, error) {
	if project {
		return recordChecksum(record, true, stringField(record, "name"), stringField(record, "description"), stringField(record, "status"), workFields(record, true))
	}
	return recordChecksum(record, false, stringField(record, "title"), issueBody(record), stringField(record, "status"), workFields(record, false))
}

func targetNodeChecksum(fieldsJSON []byte, title, body, state string, project bool) (string, error) {
	var fields Record
	if err := decodeExactJSON(fieldsJSON, &fields); err != nil {
		return "", err
	}
	classic, _ := fields["classic"].(map[string]any)
	if classic == nil {
		return "", errors.New("imported node lacks classic provenance")
	}
	return recordChecksum(Record(classic), project, title, body, state, workFields(fields, project))
}

func itemID(record Record, category string) (string, error) {
	if id, ok := intField(record, "id"); ok {
		return strconv.FormatInt(id, 10), nil
	}
	if category == "relations" {
		source, sourceOK := intField(record, "source_id")
		target, targetOK := intField(record, "target_id")
		if sourceOK && targetOK && stringField(record, "type") != "" {
			return fmt.Sprintf("%s:%d:%d", stringField(record, "type"), source, target), nil
		}
	}
	return "", fmt.Errorf("%s record lacks classic id", category)
}

func projectOf(issue Record) int64 {
	id, _ := intField(issue, "project_id")
	return id
}

// Reconcile reads a full classic snapshot and attachment bytes with GET only,
// then compares it to one Aeon tenant. It does not modify either system. The
// coordinator can expose it as `aeon import reconcile --source-url URL
// --api-key-file FILE --tenant SLUG` and encode the returned report as JSON;
// Summary is the accompanying short human-readable line.
func Reconcile(ctx context.Context, source *HTTPSource, pool *pgxpool.Pool, store attachments.Store, tenant, project string) (ReconcileReport, error) {
	if source == nil || pool == nil {
		return ReconcileReport{}, errors.New("source and database pool are required")
	}
	snap, err := source.Read(ctx, project)
	if err != nil {
		return ReconcileReport{}, err
	}
	if snap.SourceID != source.InstanceID() {
		return ReconcileReport{}, errors.New("source identity mismatch")
	}
	if len(snap.Skipped) != 0 {
		return ReconcileReport{}, fmt.Errorf("classic snapshot incomplete: %d skipped items", len(snap.Skipped))
	}
	sourceSet, targetSet := reconcileSet{}, reconcileSet{}
	projectKeys := map[int64]string{}
	for _, p := range snap.Projects {
		pid, ok := intField(p.Record, "id")
		if !ok {
			return ReconcileReport{}, errors.New("project missing id")
		}
		projectKeys[pid] = stringField(p.Record, "key")
		hash, err := sourceNodeChecksum(p.Record, true)
		if err != nil {
			return ReconcileReport{}, err
		}
		sourceSet.put(pid, "projects", strconv.FormatInt(pid, 10), hash)
		for _, issue := range p.Issues {
			if err := addSourceIssue(sourceSet, pid, issue); err != nil {
				return ReconcileReport{}, err
			}
		}
	}
	if project == "" {
		for _, issue := range snap.Orphans {
			if err := addSourceIssue(sourceSet, 0, issue); err != nil {
				return ReconcileReport{}, err
			}
		}
		if len(snap.Orphans) > 0 {
			projectKeys[0] = "(orphans)"
		}
	}
	issueProjects := map[int64]int64{}
	for _, p := range snap.Projects {
		pid, _ := intField(p.Record, "id")
		for _, issue := range p.Issues {
			iid, _ := intField(issue, "id")
			issueProjects[iid] = pid
			addSourcePeople(sourceSet, pid, iid, issue, "assignee_id", "created_by", "accepted_by")
		}
		owner, ok := intField(p.Record, "product_owner")
		if ok {
			sourceSet.put(pid, "people_links", fmt.Sprintf("project:%d:product_owner", pid), strconv.FormatInt(owner, 10))
		}
	}
	for _, issue := range snap.Orphans {
		iid, _ := intField(issue, "id")
		issueProjects[iid] = 0
		addSourcePeople(sourceSet, 0, iid, issue, "assignee_id", "created_by", "accepted_by")
	}
	for iid, details := range snap.Details {
		pid := issueProjects[iid]
		for _, group := range []struct {
			name string
			rows []Record
		}{{"comments", details.Comments}, {"relations", details.Relations}} {
			for _, record := range group.rows {
				id, err := itemID(record, group.name)
				if err != nil {
					return ReconcileReport{}, err
				}
				hash, err := checksum(record)
				if err != nil {
					return ReconcileReport{}, err
				}
				sourceSet.put(pid, group.name, id, hash)
			}
		}
		for _, record := range details.Attachments {
			id, err := itemID(record, "attachments")
			if err != nil {
				return ReconcileReport{}, err
			}
			attachmentID, _ := strconv.ParseInt(id, 10, 64)
			body, err := source.downloadAttachment(ctx, attachmentID)
			if isNotFound(err) {
				sourceSet.put(pid, "attachments", id, "source-file-missing")
				continue
			}
			if err != nil {
				return ReconcileReport{}, err
			}
			h := sha256.New()
			_, copyErr := io.Copy(h, body)
			closeErr := body.Close()
			if copyErr != nil {
				return ReconcileReport{}, fmt.Errorf("hash classic attachment %s: %w", id, copyErr)
			}
			if closeErr != nil {
				return ReconcileReport{}, closeErr
			}
			sourceSet.put(pid, "attachments", id, hex.EncodeToString(h.Sum(nil)))
		}
	}
	tenantID, err := tenantbootstrap.ResolveSlug(ctx, pool, tenant)
	if err != nil {
		return ReconcileReport{}, fmt.Errorf("resolve tenant: %w", err)
	}
	err = db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		return readReconcileTarget(ctx, tx, tenantID, snap.SourceID, project, projectKeys, store, targetSet)
	})
	if err != nil {
		return ReconcileReport{}, err
	}
	return finishReconcile(snap.SourceID, tenant, sourceSet, targetSet, projectKeys), nil
}

func addSourceIssue(set reconcileSet, pid int64, issue Record) error {
	id, ok := intField(issue, "id")
	if !ok {
		return errors.New("issue missing id")
	}
	category := "tickets"
	if isKnowledge(stringField(issue, "type")) {
		category = "knowledge"
	}
	hash, err := sourceNodeChecksum(issue, false)
	if err == nil {
		set.put(pid, category, strconv.FormatInt(id, 10), hash)
	}
	return err
}

func isKnowledge(kind string) bool {
	switch canonicalType(kind) {
	case "memory", "runbook", "guideline", "external_system", "related_project":
		return true
	}
	return false
}

func addSourcePeople(set reconcileSet, pid, iid int64, issue Record, fields ...string) {
	for _, field := range fields {
		if userID, ok := intField(issue, field); ok {
			set.put(pid, "people_links", fmt.Sprintf("issue:%d:%s", iid, field), strconv.FormatInt(userID, 10))
		}
	}
}

type targetNode struct {
	ID, Key, Kind, Title, Body, State, ParentID string
	Fields                                      []byte
	Project, ClassicID                          int64
}

func readReconcileTarget(ctx context.Context, tx pgx.Tx, tenantID, sourceID, selectedProject string, projectKeys map[int64]string, store attachments.Store, target reconcileSet) error {
	rows, err := tx.Query(ctx, `SELECT n.id::text,n.key,k.slug,n.title,n.body,n.state,n.fields,coalesce(n.parent_id::text,'')
	 FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
	 WHERE n.tenant_id=$1 AND n.fields->'classic'->>'source_id'=$2`, tenantID, sourceID)
	if err != nil {
		return err
	}
	nodes := map[string]targetNode{}
	for rows.Next() {
		var n targetNode
		if err := rows.Scan(&n.ID, &n.Key, &n.Kind, &n.Title, &n.Body, &n.State, &n.Fields, &n.ParentID); err != nil {
			rows.Close()
			return err
		}
		var fields Record
		if err := decodeExactJSON(n.Fields, &fields); err != nil {
			rows.Close()
			return err
		}
		classic, _ := fields["classic"].(map[string]any)
		n.ClassicID, _ = intField(Record(classic), "id")
		if n.Kind == "project" {
			n.Project = n.ClassicID
		} else {
			n.Project, _ = intField(Record(classic), "project_id")
		}
		if selectedProject != "" {
			if _, included := projectKeys[n.Project]; !included {
				continue
			}
		}
		nodes[n.ID] = n
		if _, found := projectKeys[n.Project]; !found {
			if n.Project == 0 {
				projectKeys[0] = "(orphans)"
			} else {
				projectKeys[n.Project] = "PRJ-" + strconv.FormatInt(n.Project, 10)
			}
		}
		category := "tickets"
		if n.Kind == "project" {
			category = "projects"
			if _, found := projectKeys[n.Project]; !found {
				projectKeys[n.Project] = n.Key
			}
		} else if isKnowledge(n.Kind) {
			category = "knowledge"
		}
		hash, err := targetNodeChecksum(n.Fields, n.Title, n.Body, n.State, n.Kind == "project")
		if err != nil {
			rows.Close()
			return err
		}
		target.put(n.Project, category, strconv.FormatInt(n.ClassicID, 10), hash)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	byClassicID := map[int64]string{}
	for _, node := range nodes {
		if node.Kind != "project" {
			byClassicID[node.ClassicID] = node.ID
		}
	}
	nativeRelations := map[string]bool{}
	links, err := tx.Query(ctx, `SELECT source_node_id::text,target_node_id::text,type FROM node_relations WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return err
	}
	for links.Next() {
		var sourceNode, targetNode, typ string
		if err := links.Scan(&sourceNode, &targetNode, &typ); err != nil {
			links.Close()
			return err
		}
		nativeRelations[sourceNode+":"+targetNode+":"+typ] = true
	}
	if err := links.Err(); err != nil {
		links.Close()
		return err
	}
	links.Close()
	// A linked principal has a different UUID from its classic identity row.
	// Compare the resolved Aeon assignment to its canonical classic user ID.
	userByPrincipal := map[string]string{}
	users, err := tx.Query(ctx, `SELECT coalesce(p.linked_to,p.id)::text,i.subject FROM principals p JOIN identities i ON i.id=p.identity_id WHERE p.tenant_id=$1 AND i.issuer='paimos-classic' AND starts_with(i.subject,$2)`, tenantID, sourceID+":")
	if err != nil {
		return err
	}
	for users.Next() {
		var principal, subject string
		if err := users.Scan(&principal, &subject); err != nil {
			users.Close()
			return err
		}
		userByPrincipal[principal] = strings.TrimPrefix(subject, sourceID+":")
	}
	if err := users.Err(); err != nil {
		users.Close()
		return err
	}
	users.Close()
	for _, n := range nodes {
		var fields Record
		if err := decodeExactJSON(n.Fields, &fields); err != nil {
			return err
		}
		classic := fields["classic"].(map[string]any)
		links := map[string]string{"assignee_id": "assignee", "created_by": "created_by", "accepted_by": "accepted_by"}
		prefix := "issue"
		if n.Kind == "project" {
			links = map[string]string{"product_owner": "product_owner"}
			prefix = "project"
		}
		for sourceField, targetField := range links {
			if _, exists := classic[sourceField]; !exists {
				continue
			}
			principal := stringField(fields, targetField)
			if principal != "" {
				target.put(n.Project, "people_links", fmt.Sprintf("%s:%d:%s", prefix, n.ClassicID, sourceField), userByPrincipal[principal])
			}
		}
	}
	query := `SELECT e.node_id::text,e.type,e.after FROM events e WHERE e.tenant_id=$1 AND e.type IN ('import.comment','import.relation') AND e.after->>'classic_ref' LIKE $2 ORDER BY e.id`
	eventsRows, err := tx.Query(ctx, query, tenantID, sourceID+":%")
	if err != nil {
		return err
	}
	for eventsRows.Next() {
		var nodeID, typ string
		var raw []byte
		if err := eventsRows.Scan(&nodeID, &typ, &raw); err != nil {
			eventsRows.Close()
			return err
		}
		node, exists := nodes[nodeID]
		if !exists {
			continue
		}
		var payload struct {
			Record Record `json:"record"`
		}
		if err := decodeExactJSON(raw, &payload); err != nil {
			eventsRows.Close()
			return err
		}
		category := strings.TrimPrefix(typ, "import.") + "s"
		id, err := itemID(payload.Record, category)
		if err != nil {
			eventsRows.Close()
			return err
		}
		hash, err := checksum(payload.Record)
		if err != nil {
			eventsRows.Close()
			return err
		}
		if category == "relations" && !relationPresent(payload.Record, nodes, byClassicID, nativeRelations) {
			hash += ":projection-missing"
		}
		target.put(node.Project, category, id, hash)
	}
	if err := eventsRows.Err(); err != nil {
		eventsRows.Close()
		return err
	}
	eventsRows.Close()
	attachments, err := tx.Query(ctx, `SELECT a.node_id::text,a.source_attachment_id,a.sha256 FROM attachments a WHERE a.tenant_id=$1 AND a.source_id=$2 AND a.deleted_at IS NULL`, tenantID, sourceID)
	if err != nil {
		return err
	}
	for attachments.Next() {
		var nodeID, sha string
		var classicID int64
		if err := attachments.Scan(&nodeID, &classicID, &sha); err != nil {
			attachments.Close()
			return err
		}
		if n, found := nodes[nodeID]; found {
			file, err := store.Open(tenantID, sha, "original")
			if errors.Is(err, os.ErrNotExist) {
				target.put(n.Project, "attachments", strconv.FormatInt(classicID, 10), "aeon-file-missing")
				continue
			}
			if err != nil {
				attachments.Close()
				return err
			}
			h := sha256.New()
			_, copyErr := io.Copy(h, file)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				attachments.Close()
				return fmt.Errorf("hash Aeon attachment %d: %v %v", classicID, copyErr, closeErr)
			}
			target.put(n.Project, "attachments", strconv.FormatInt(classicID, 10), hex.EncodeToString(h.Sum(nil)))
		}
	}
	if err := attachments.Err(); err != nil {
		attachments.Close()
		return err
	}
	attachments.Close()
	return nil
}

func relationPresent(record Record, nodes map[string]targetNode, byClassicID map[int64]string, native map[string]bool) bool {
	typ := stringField(record, "type")
	sourceID, sourceOK := intField(record, "source_id")
	targetID, targetOK := intField(record, "target_id")
	if !sourceOK || !targetOK {
		return true // Unsupported source rows are retained as events only.
	}
	sourceNode, targetNode := byClassicID[sourceID], byClassicID[targetID]
	if sourceNode == "" || targetNode == "" {
		return true // Out-of-scope links are retained as events only.
	}
	if typ == "parent" {
		return nodes[targetNode].ParentID == sourceNode
	}
	mapped, mode, ok := mapClassicRelation(typ)
	if !ok || sourceNode == targetNode {
		return true
	}
	if mode == "dependency" {
		sourceNode, targetNode = targetNode, sourceNode
	}
	if mode == "undirected" && sourceNode > targetNode {
		sourceNode, targetNode = targetNode, sourceNode
	}
	return native[sourceNode+":"+targetNode+":"+mapped]
}

func setChecksum(records map[string]string) string {
	ids := make([]string, 0, len(records))
	for id := range records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	h := sha256.New()
	for _, id := range ids {
		_, _ = io.WriteString(h, id+"\x00"+records[id]+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func finishReconcile(sourceID, tenant string, source, target reconcileSet, keys map[int64]string) ReconcileReport {
	report := ReconcileReport{SourceID: sourceID, Tenant: tenant, Projects: []ReconcileProject{}}
	ids := make([]int64, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var missing, extra, changed int
	for _, pid := range ids {
		p := ReconcileProject{ClassicID: pid, Key: keys[pid], Categories: map[string]ReconcileCategory{}}
		for _, category := range []string{"projects", "tickets", "knowledge", "comments", "relations", "attachments", "people_links"} {
			s, t := source[pid][category], target[pid][category]
			c := ReconcileCategory{SourceCount: len(s), AeonCount: len(t), SourceChecksum: setChecksum(s), AeonChecksum: setChecksum(t), Missing: []ReconcileDifference{}, Extra: []ReconcileDifference{}, Changed: []ReconcileDifference{}}
			for id, sourceHash := range s {
				if targetHash, found := t[id]; !found {
					c.Missing = append(c.Missing, ReconcileDifference{ClassicID: id, SourceChecksum: sourceHash})
				} else if targetHash != sourceHash {
					c.Changed = append(c.Changed, ReconcileDifference{ClassicID: id, SourceChecksum: sourceHash, AeonChecksum: targetHash})
				}
			}
			for id, targetHash := range t {
				if _, found := s[id]; !found {
					c.Extra = append(c.Extra, ReconcileDifference{ClassicID: id, AeonChecksum: targetHash})
				}
			}
			sort.Slice(c.Missing, func(i, j int) bool { return c.Missing[i].ClassicID < c.Missing[j].ClassicID })
			sort.Slice(c.Extra, func(i, j int) bool { return c.Extra[i].ClassicID < c.Extra[j].ClassicID })
			sort.Slice(c.Changed, func(i, j int) bool { return c.Changed[i].ClassicID < c.Changed[j].ClassicID })
			missing += len(c.Missing)
			extra += len(c.Extra)
			changed += len(c.Changed)
			p.Categories[category] = c
		}
		report.Projects = append(report.Projects, p)
	}
	report.Summary = fmt.Sprintf("%d projects checked; %d missing, %d extra, %d changed classic items", len(report.Projects), missing, extra, changed)
	return report
}
