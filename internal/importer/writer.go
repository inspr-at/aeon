// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresWriter targets the R1 nodes, relations and events schema.
type PostgresWriter struct{ Pool *pgxpool.Pool }

func (w PostgresWriter) Write(ctx context.Context, s Snapshot, tenantSlug string) (Report, error) {
	r := Analyze(s)
	if w.Pool == nil {
		return r, errors.New("database pool is required")
	}
	if s.SourceID == "" {
		return r, errors.New("source identity is required")
	}
	var tenantID string
	if err := w.Pool.QueryRow(ctx, `SELECT id FROM tenants WHERE slug=$1`, tenantSlug).Scan(&tenantID); err != nil {
		return r, fmt.Errorf("resolve tenant: %w", err)
	}
	err := db.InTenant(ctx, w.Pool, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,42))`, tenantID+":"+s.SourceID); err != nil {
			return err
		}
		actor, users, err := importUsers(ctx, tx, tenantID, s)
		if err != nil {
			return err
		}
		kinds := map[string]string{}
		kindID := func(slug string) (string, error) {
			if id := kinds[slug]; id != "" {
				return id, nil
			}
			var id string
			prefix := map[string]string{"project": "PRJ", "epic": "EPC", "ticket": "TKT", "task": "TSK", "release": "REL", "sprint": "SPR", "cost_unit": "CU", "memory": "MEM", "runbook": "RUN", "guideline": "GUI", "external_system": "EXT", "related_project": "RPR"}[slug]
			if prefix == "" {
				return "", fmt.Errorf("unsupported classic issue type %q", slug)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,slug) DO NOTHING`, tenantID, slug, strings.ReplaceAll(slug, "_", " "), prefix, slug); err != nil {
				return "", err
			}
			if err := tx.QueryRow(ctx, `SELECT id FROM node_kinds WHERE tenant_id=$1 AND slug=$2`, tenantID, slug).Scan(&id); err != nil {
				return "", err
			}
			kinds[slug] = id
			return id, nil
		}
		projectIDs := map[int64]string{}
		issueIDs := map[int64]string{}
		issueRows := map[int64]Record{}
		for _, p := range s.Projects {
			pid, _ := intField(p.Record, "id")
			kind, err := kindID("project")
			if err != nil {
				return err
			}
			key := "PRJ-" + strconv.FormatInt(pid, 10)
			id, created, updated, err := upsertNode(ctx, tx, tenantID, s.SourceID, kind, key, stringField(p.Record, "name"), stringField(p.Record, "description"), stringField(p.Record, "status"), "", p.Record, principalRefs(p.Record, users, "product_owner"), actor, true)
			if err != nil {
				return fmt.Errorf("project %d: %w", pid, err)
			}
			projectIDs[pid] = id
			if created {
				r.Created++
			}
			if updated {
				r.Updated++
			}
		}
		for _, item := range s.Orphans {
			iid, ok := intField(item, "id")
			if !ok {
				return errors.New("orphan issue missing id")
			}
			key := stringField(item, "issue_key")
			if key == "" {
				if stringField(item, "type") != "sprint" {
					return fmt.Errorf("orphan issue %d missing issue_key", iid)
				}
				key = "SPRINT-" + strconv.FormatInt(iid, 10)
			}
			kind, err := kindID(canonicalType(stringField(item, "type")))
			if err != nil {
				return err
			}
			id, created, updated, err := upsertNode(ctx, tx, tenantID, s.SourceID, kind, key, stringField(item, "title"), issueBody(item), stringField(item, "status"), "", item, principalRefs(item, users, "assignee_id", "created_by", "accepted_by", "deleted_by"), actor, false)
			if err != nil {
				return fmt.Errorf("orphan issue %d: %w", iid, err)
			}
			issueIDs[iid] = id
			issueRows[iid] = item
			if created {
				r.Created++
			}
			if updated {
				r.Updated++
			}
		}
		for _, p := range s.Projects {
			pid, _ := intField(p.Record, "id")
			for _, item := range p.Issues {
				iid, ok := intField(item, "id")
				if !ok {
					return errors.New("issue missing id")
				}
				key := stringField(item, "issue_key")
				if key == "" {
					return fmt.Errorf("issue %d missing issue_key", iid)
				}
				kind, err := kindID(canonicalType(stringField(item, "type")))
				if err != nil {
					return err
				}
				id, created, updated, err := upsertNode(ctx, tx, tenantID, s.SourceID, kind, key, stringField(item, "title"), issueBody(item), stringField(item, "status"), projectIDs[pid], item, principalRefs(item, users, "assignee_id", "created_by", "accepted_by", "deleted_by"), actor, false)
				if err != nil {
					return fmt.Errorf("issue %s: %w", key, err)
				}
				issueIDs[iid] = id
				issueRows[iid] = item
				if created {
					r.Created++
				}
				if updated {
					r.Updated++
				}
			}
		}
		// Resolve classic issue parents after every node exists. Project parent is
		// the fallback for missing or out-of-scope ancestors.
		for iid, item := range issueRows {
			parentSource, ok := intField(item, "parent_id")
			if !ok {
				continue
			}
			parentID := issueIDs[parentSource]
			if parentID == "" {
				continue
			}
			if err := setParent(ctx, tx, tenantID, actor, issueIDs[iid], parentID); err != nil {
				return fmt.Errorf("parent for %d: %w", iid, err)
			}
		}
		for iid, d := range s.Details {
			nodeID := issueIDs[iid]
			if nodeID == "" {
				continue
			}
			for _, rel := range d.Relations {
				sourceID, _ := intField(rel, "source_id")
				targetID, _ := intField(rel, "target_id")
				typ := stringField(rel, "type")
				if err := importEvent(ctx, tx, tenantID, actor, nodeID, "import.relation", s.SourceID, rel, "id", typ+":"+strconv.FormatInt(sourceID, 10)+":"+strconv.FormatInt(targetID, 10)); err != nil {
					return err
				}
				src, dst := issueIDs[sourceID], issueIDs[targetID]
				if src == "" || dst == "" {
					continue
				}
				if typ == "parent" {
					if err := setParent(ctx, tx, tenantID, actor, dst, src); err != nil {
						return err
					}
					continue
				}
				mapped := map[string]string{"depends_on": "blocks", "relates": "relates", "duplicates": "duplicates", "cites": "cites"}[typ]
				if mapped != "" {
					if typ == "depends_on" {
						src, dst = dst, src // dependency blocks dependent
					}
					if mapped == "relates" && src > dst {
						src, dst = dst, src
					}
					if src != dst {
						if _, err := tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,source_node_id,target_node_id,type) DO NOTHING`, tenantID, src, dst, mapped); err != nil {
							return err
						}
					}
				}
			}
			for _, group := range []struct {
				typ  string
				rows []Record
			}{{"import.comment", d.Comments}, {"import.history", d.History}, {"import.attachment", d.Attachments}} {
				for _, record := range group.rows {
					if err := importEvent(ctx, tx, tenantID, actor, nodeID, group.typ, s.SourceID, record, "id", ""); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return r, err
}

func importUsers(ctx context.Context, tx pgx.Tx, tenantID string, s Snapshot) (string, map[int64]string, error) {
	users := map[int64]string{}
	for _, u := range s.Users {
		id, ok := intField(u, "id")
		if !ok {
			return "", nil, errors.New("user missing id")
		}
		subject := s.SourceID + ":" + strconv.FormatInt(id, 10)
		name := stringField(u, "username")
		if name == "" {
			return "", nil, fmt.Errorf("user %d missing username", id)
		}
		var identityID, principalID string
		createdAt := parseClassicTime(stringField(u, "created_at"))
		if err := tx.QueryRow(ctx, `INSERT INTO identities(issuer,subject,email,display_name,created_at) VALUES('paimos-classic',$1,$2,$3,coalesce($4::timestamptz,now())) ON CONFLICT(issuer,subject) DO UPDATE SET email=EXCLUDED.email,display_name=EXCLUDED.display_name RETURNING id`, subject, nullString(stringField(u, "email")), name, createdAt).Scan(&identityID); err != nil {
			return "", nil, err
		}
		roles := []string{stringField(u, "role")}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,identity_id,name,roles,created_at) VALUES($1,'person',$2,$3,$4,coalesce($5::timestamptz,now())) ON CONFLICT(tenant_id,identity_id) WHERE identity_id IS NOT NULL DO UPDATE SET name=EXCLUDED.name,roles=EXCLUDED.roles RETURNING id`, tenantID, identityID, name, roles, createdAt).Scan(&principalID); err != nil {
			return "", nil, err
		}
		users[id] = principalID
	}
	var actor string
	err := tx.QueryRow(ctx, `SELECT id FROM principals WHERE tenant_id=$1 AND kind='agent' AND name='Classic Paimos importer' ORDER BY created_at LIMIT 1`, tenantID).Scan(&actor)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1,'agent','Classic Paimos importer',ARRAY['importer']) RETURNING id`, tenantID).Scan(&actor)
	}
	return actor, users, err
}

func upsertNode(ctx context.Context, tx pgx.Tx, tenantID, sourceID, kindID, key, title, body, state, parentID string, original, refs Record, actor string, project bool) (string, bool, bool, error) {
	if title == "" {
		return "", false, false, errors.New("title is empty")
	}
	if state == "" {
		state = "open"
	}
	fields := mappedFields(original, refs, sourceID, project)
	bodyJSON, err := jsonValue(fields)
	if err != nil {
		return "", false, false, err
	}
	var id, oldTitle, oldBody, oldState string
	var oldFields, beforeJSON []byte
	err = tx.QueryRow(ctx, `SELECT id,title,body,state,fields,to_jsonb(nodes) FROM nodes WHERE tenant_id=$1 AND key=$2`, tenantID, key).Scan(&id, &oldTitle, &oldBody, &oldState, &oldFields, &beforeJSON)
	created := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !created {
		return "", false, false, err
	}
	if !created {
		var old map[string]any
		if err := json.Unmarshal(oldFields, &old); err != nil {
			return "", false, false, err
		}
		classic, _ := old["classic"].(map[string]any)
		if classic["source_id"] != sourceID {
			return "", false, false, fmt.Errorf("key %s belongs to another source", key)
		}
		var now any
		var prior any
		if err := json.Unmarshal(bodyJSON, &now); err != nil {
			return "", false, false, err
		}
		if err := json.Unmarshal(oldFields, &prior); err != nil {
			return "", false, false, err
		}
		if oldTitle == title && oldBody == body && oldState == state && reflect.DeepEqual(prior, now) {
			return id, false, false, nil
		}
		_, err = tx.Exec(ctx, `UPDATE nodes SET title=$3,body=$4,state=$5,fields=$6::jsonb,updated_at=coalesce($7::timestamptz,now()) WHERE tenant_id=$1 AND id=$2`, tenantID, id, title, body, state, string(bodyJSON), parseClassicTime(stringField(original, "updated_at")))
		if err != nil {
			return "", false, false, err
		}
	} else {
		createdAt := parseClassicTime(stringField(original, "created_at"))
		updatedAt := parseClassicTime(stringField(original, "updated_at"))
		err = tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,body,state,parent_id,fields,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,coalesce($9::timestamptz,now()),coalesce($10::timestamptz,now())) RETURNING id`, tenantID, key, kindID, title, body, state, nullString(parentID), string(bodyJSON), createdAt, updatedAt).Scan(&id)
		if err != nil {
			return "", false, false, err
		}
	}
	var afterJSON []byte
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(nodes) FROM nodes WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&afterJSON); err != nil {
		return "", false, false, err
	}
	_, err = events.Append(ctx, tx, tenant.Principal{TenantID: tenantID, ID: actor}, events.Change{
		NodeID: &id, Type: map[bool]string{true: "import.node_created", false: "import.node_updated"}[created],
		Before: rawSnapshot(beforeJSON), After: json.RawMessage(afterJSON),
	})
	return id, created, !created, err
}

func rawSnapshot(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

func setParent(ctx context.Context, tx pgx.Tx, tenantID, actor, childID, parentID string) error {
	var before, after []byte
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(nodes) FROM nodes WHERE tenant_id=$1 AND id=$2`, tenantID, childID).Scan(&before); err != nil {
		return err
	}
	changed, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=$3 WHERE tenant_id=$1 AND id=$2 AND parent_id IS DISTINCT FROM $3`, tenantID, childID, parentID)
	if err != nil {
		return err
	}
	if changed.RowsAffected() == 0 {
		return nil
	}
	if err := tx.QueryRow(ctx, `SELECT to_jsonb(nodes) FROM nodes WHERE tenant_id=$1 AND id=$2`, tenantID, childID).Scan(&after); err != nil {
		return err
	}
	_, err = events.Append(ctx, tx, tenant.Principal{TenantID: tenantID, ID: actor}, events.Change{
		NodeID: &childID, Type: "import.parent_changed",
		Before: json.RawMessage(before), After: json.RawMessage(after),
	})
	return err
}

func importEvent(ctx context.Context, tx pgx.Tx, tenantID, actor, nodeID, typ, sourceID string, record Record, idField, suffix string) error {
	id, ok := intField(record, idField)
	ref := sourceID + ":" + typ + ":" + strconv.FormatInt(id, 10)
	if !ok {
		if suffix == "" {
			return fmt.Errorf("%s record missing id", typ)
		}
		ref = sourceID + ":" + typ + ":" + suffix
	}
	// Classic comments and attachment metadata can be edited. Record each
	// distinct revision once while leaving Aeon's event log append-only.
	if typ == "import.comment" || typ == "import.attachment" {
		canonical, err := jsonValue(record)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(canonical)
		ref += fmt.Sprintf(":%x", hash[:16])
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE tenant_id=$1 AND type=$2 AND after ? 'classic_ref' AND after->>'classic_ref'=$3)`, tenantID, typ, ref).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	payload := Record{"classic_ref": ref, "record": record}
	at := classicTime(stringField(record, "created_at"))
	if at == nil {
		at = classicTime(stringField(record, "changed_at"))
	}
	_, err := events.Append(ctx, tx, tenant.Principal{TenantID: tenantID, ID: actor}, events.Change{
		NodeID: &nodeID, Type: typ, After: payload, At: at,
	})
	return err
}
func issueBody(r Record) string {
	if v := stringField(r, "description"); v != "" {
		return v
	}
	return stringField(r, "body")
}
func principalRefs(r Record, users map[int64]string, fields ...string) Record {
	refs := Record{}
	for _, field := range fields {
		if id, ok := intField(r, field); ok && users[id] != "" {
			refs[field] = users[id]
		}
	}
	return refs
}
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func parseClassicTime(s string) any {
	return classicTime(s)
}
func classicTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
