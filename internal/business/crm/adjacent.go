// SPDX-License-Identifier: AGPL-3.0-only
package crm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/business/quotes"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/jackc/pgx/v5"
)

// ReformatCustomerNumber delegates the guarded multi-quote conversion to QP1.
// Both modules use crm_customer_numbers; no second allocator is introduced.
func (m *module) reformatNumber(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, true)
	if !ok {
		return
	}
	id, e := pathUUID(r, "organisationId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var in struct {
		ExpectedCustomerNo string `json:"expected_customer_no"`
	}
	if e = decodeCRM(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	number, e := quotes.ReformatLegacyCustomerNumber(r.Context(), m.pool, m.reg, p, id, in.ExpectedCustomerNo)
	if e != nil {
		writeErr(w, errConflict)
		return
	}
	httpapi.WriteJSON(w, 200, map[string]string{"customer_no": number})
}

type RelatedNode struct {
	ID                  string          `json:"id"`
	Key                 string          `json:"key"`
	Title               string          `json:"title"`
	State               string          `json:"state"`
	Cooperation         json.RawMessage `json:"cooperation"`
	CooperationRevision int64           `json:"cooperation_revision"`
}
type RelatedQuote struct {
	ID       string  `json:"id"`
	OfferNo  *string `json:"offer_no"`
	State    string  `json:"state"`
	Archived bool    `json:"archived"`
}
type RelatedHours struct {
	ProjectNodeID   string `json:"project_node_id"`
	Currency        string `json:"currency"`
	DurationSeconds int64  `json:"duration_seconds"`
	Amount          string `json:"amount"`
}
type Document struct {
	AttachmentID string  `json:"attachment_id"`
	NodeID       string  `json:"node_id"`
	Name         string  `json:"name"`
	Title        string  `json:"title"`
	Category     string  `json:"category"`
	Status       string  `json:"status"`
	ValidFrom    *string `json:"valid_from"`
	ValidUntil   *string `json:"valid_until"`
	Revision     int64   `json:"revision"`
}
type Related struct {
	Projects  []RelatedNode  `json:"projects"`
	Quotes    []RelatedQuote `json:"quotes"`
	Hours     []RelatedHours `json:"hours"`
	Documents []Document     `json:"documents"`
}

func (m *module) related(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, false)
	if !ok {
		return
	}
	id, e := pathUUID(r, "organisationId")
	if e != nil {
		writeErr(w, e)
		return
	}
	out := Related{Projects: []RelatedNode{}, Quotes: []RelatedQuote{}, Hours: []RelatedHours{}, Documents: []Document{}}
	e = m.run(r, p, fence.PermViewsProvide, func(tx pgx.Tx) error {
		if _, e := customer(r.Context(), tx, id, false); e != nil {
			return e
		}
		ctx := r.Context()
		rows, e := tx.Query(ctx, `SELECT n.id::text,n.key,n.title,n.state,coalesce(c.data,'{}'::jsonb),coalesce(c.revision,0) FROM node_relations x JOIN nodes n ON n.tenant_id=x.tenant_id AND n.id=x.target_node_id JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id LEFT JOIN crm_project_cooperation c ON c.tenant_id=n.tenant_id AND c.project_node_id=n.id WHERE x.source_node_id=$1::uuid AND x.type='customer_of' AND k.slug='project' AND n.deleted_at IS NULL ORDER BY n.title,n.id`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v RelatedNode
			if e = rows.Scan(&v.ID, &v.Key, &v.Title, &v.State, &v.Cooperation, &v.CooperationRevision); e != nil {
				break
			}
			out.Projects = append(out.Projects, v)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `SELECT q.quote_node_id::text,q.offer_no,q.state,q.archived_at IS NOT NULL FROM business_quotes q WHERE q.customer_org_node_id=$1::uuid AND q.deleted_at IS NULL ORDER BY q.created_at DESC,q.quote_node_id`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v RelatedQuote
			if e = rows.Scan(&v.ID, &v.OfferNo, &v.State, &v.Archived); e != nil {
				break
			}
			out.Quotes = append(out.Quotes, v)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `WITH RECURSIVE project_tree(root_id,node_id) AS (
		 SELECT r.target_node_id,r.target_node_id FROM node_relations r JOIN nodes n ON n.tenant_id=r.tenant_id AND n.id=r.target_node_id WHERE r.tenant_id=$2::uuid AND r.source_node_id=$1::uuid AND r.type='customer_of' AND n.deleted_at IS NULL
		 UNION ALL SELECT t.root_id,n.id FROM project_tree t JOIN nodes n ON n.tenant_id=$2::uuid AND n.parent_id=t.node_id AND n.deleted_at IS NULL
		) SELECT t.root_id::text,e.currency,sum(e.duration_seconds)::bigint,sum(e.amount)::text FROM project_tree t JOIN time_entries e ON e.tenant_id=$2::uuid AND e.node_id=t.node_id GROUP BY t.root_id,e.currency ORDER BY t.root_id,e.currency`, id, p.TenantID)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v RelatedHours
			if e = rows.Scan(&v.ProjectNodeID, &v.Currency, &v.DurationSeconds, &v.Amount); e != nil {
				break
			}
			out.Hours = append(out.Hours, v)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `SELECT a.id::text,a.node_id::text,a.name,coalesce(d.title,''),coalesce(d.category,''),coalesce(d.status,'draft'),d.valid_from::text,d.valid_until::text,coalesce(d.revision,0) FROM attachments a LEFT JOIN crm_document_metadata d ON d.tenant_id=a.tenant_id AND d.attachment_id=a.id WHERE a.deleted_at IS NULL AND (a.node_id=$1::uuid OR a.node_id IN(SELECT target_node_id FROM node_relations WHERE source_node_id=$1::uuid AND type='customer_of')) ORDER BY a.created_at DESC,a.id`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v Document
			if e = rows.Scan(&v.AttachmentID, &v.NodeID, &v.Name, &v.Title, &v.Category, &v.Status, &v.ValidFrom, &v.ValidUntil, &v.Revision); e != nil {
				break
			}
			out.Documents = append(out.Documents, v)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		return e
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 200, out)
}

type DocumentMetadata struct {
	AttachmentID     string  `json:"attachment_id"`
	Title            string  `json:"title"`
	Category         string  `json:"category"`
	Status           string  `json:"status"`
	ValidFrom        *string `json:"valid_from"`
	ValidUntil       *string `json:"valid_until"`
	ExpectedRevision int64   `json:"expected_revision"`
	Revision         int64   `json:"revision"`
}

func dateOK(s *string) bool {
	if s == nil {
		return true
	}
	_, e := time.Parse("2006-01-02", *s)
	return e == nil
}
func (m *module) putDocumentMetadata(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, true)
	if !ok {
		return
	}
	id, e := pathUUID(r, "attachmentId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var in DocumentMetadata
	if e = decodeCRM(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	if len(in.Title) > 255 || len(in.Category) > 100 || in.Status != "draft" && in.Status != "active" && in.Status != "expired" || !dateOK(in.ValidFrom) || !dateOK(in.ValidUntil) || in.ValidFrom != nil && in.ValidUntil != nil && *in.ValidFrom > *in.ValidUntil {
		writeErr(w, errInvalid("invalid metadata"))
		return
	}
	var out DocumentMetadata
	e = m.run(r, p, fence.PermNodesContribute, func(tx pgx.Tx) error {
		var nodeID string
		var slug string
		e := tx.QueryRow(r.Context(), `SELECT a.node_id::text,k.slug FROM attachments a JOIN nodes n ON n.tenant_id=a.tenant_id AND n.id=a.node_id JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE a.id=$1::uuid AND a.deleted_at IS NULL AND n.deleted_at IS NULL FOR UPDATE OF a`, id).Scan(&nodeID, &slug)
		if errors.Is(e, pgx.ErrNoRows) {
			return errNotFound
		}
		if e != nil {
			return e
		}
		if slug != "organisation" && slug != "project" && slug != "quote" {
			return errConflict
		}
		var before *DocumentMetadata
		var old DocumentMetadata
		e = tx.QueryRow(r.Context(), `SELECT title,category,status,valid_from::text,valid_until::text,revision FROM crm_document_metadata WHERE attachment_id=$1::uuid FOR UPDATE`, id).Scan(&old.Title, &old.Category, &old.Status, &old.ValidFrom, &old.ValidUntil, &old.Revision)
		if e == nil {
			old.AttachmentID = id
			before = &old
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if before == nil && in.ExpectedRevision != 0 || before != nil && in.ExpectedRevision != old.Revision {
			return errConflict
		}
		e = tx.QueryRow(r.Context(), `INSERT INTO crm_document_metadata(tenant_id,attachment_id,title,category,status,valid_from,valid_until) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6::date,$7::date) ON CONFLICT(tenant_id,attachment_id) DO UPDATE SET title=excluded.title,category=excluded.category,status=excluded.status,valid_from=excluded.valid_from,valid_until=excluded.valid_until,revision=crm_document_metadata.revision+1 RETURNING revision`, p.TenantID, id, in.Title, in.Category, in.Status, in.ValidFrom, in.ValidUntil).Scan(&in.Revision)
		if e != nil {
			return e
		}
		out = in
		out.AttachmentID = id
		out.ExpectedRevision = 0
		return appendCRM(r.Context(), tx, p, nodeID, "crm.document_metadata_changed", before, out)
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 200, out)
}

func liveNode(ctx context.Context, tx pgx.Tx, id, kind string, lock bool) error {
	q := `SELECT n.id FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE n.id=$1::uuid AND k.slug=$2 AND n.deleted_at IS NULL`
	if lock {
		q += ` FOR UPDATE OF n`
	}
	var got string
	e := tx.QueryRow(ctx, q, id, kind).Scan(&got)
	if errors.Is(e, pgx.ErrNoRows) {
		return errNotFound
	}
	return e
}
func (m *module) setProjectCustomer(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, true)
	if !ok {
		return
	}
	project, e := pathUUID(r, "projectId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var in struct {
		OrganisationNodeID string `json:"organisation_node_id"`
	}
	if e = decodeCRM(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	org, valid := parseUUID(in.OrganisationNodeID)
	if !valid {
		writeErr(w, errInvalid("invalid customer"))
		return
	}
	var result map[string]string
	e = m.run(r, p, fence.PermNodesContribute, func(tx pgx.Tx) error {
		if e := liveNode(r.Context(), tx, project, "project", true); e != nil {
			return e
		}
		if _, e := customer(r.Context(), tx, org, false); e != nil {
			return e
		}
		var old *string
		var current string
		e := tx.QueryRow(r.Context(), `SELECT source_node_id::text FROM node_relations WHERE target_node_id=$1::uuid AND type='customer_of' FOR UPDATE`, project).Scan(&current)
		if e == nil {
			old = &current
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if old != nil && *old == org {
			result = map[string]string{"project_node_id": project, "organisation_node_id": org}
			return nil
		}
		var conflicting bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM business_quotes WHERE project_node_id=$1::uuid AND customer_org_node_id<>$2::uuid)`, project, org).Scan(&conflicting)
		if e != nil {
			return e
		}
		if conflicting {
			return errConflict
		}
		if old != nil {
			_, e = tx.Exec(r.Context(), `DELETE FROM node_relations WHERE target_node_id=$1::uuid AND type='customer_of'`, project)
			if e != nil {
				return e
			}
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1::uuid,$2::uuid,$3::uuid,'customer_of')`, p.TenantID, org, project)
		if e != nil {
			return e
		}
		result = map[string]string{"project_node_id": project, "organisation_node_id": org}
		return appendCRM(r.Context(), tx, p, project, "crm.project_customer_changed", old, result)
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 200, result)
}

type Cooperation struct {
	Engagement                string `json:"engagement"`
	Ownership                 string `json:"ownership"`
	EnvironmentResponsibility string `json:"environment_responsibility"`
	SLA                       string `json:"sla"`
	ReportContract            string `json:"report_contract"`
	ExpectedRevision          int64  `json:"expected_revision"`
	Revision                  int64  `json:"revision"`
}

func (m *module) getCooperation(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, false)
	if !ok {
		return
	}
	id, e := pathUUID(r, "projectId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var out Cooperation
	e = m.run(r, p, fence.PermViewsProvide, func(tx pgx.Tx) error {
		if e := liveNode(r.Context(), tx, id, "project", false); e != nil {
			return e
		}
		var raw []byte
		var rev int64
		e := tx.QueryRow(r.Context(), `SELECT data,revision FROM crm_project_cooperation WHERE project_node_id=$1::uuid`, id).Scan(&raw, &rev)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &out); e != nil {
			return e
		}
		out.Revision = rev
		out.ExpectedRevision = 0
		return nil
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 200, out)
}

func (m *module) putCooperation(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, true)
	if !ok {
		return
	}
	id, e := pathUUID(r, "projectId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var in Cooperation
	if e = decodeCRM(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	if len(in.Engagement) > 500 || len(in.Ownership) > 500 || len(in.EnvironmentResponsibility) > 500 || len(in.SLA) > 5000 || len(in.ReportContract) > 5000 {
		writeErr(w, errInvalid("invalid cooperation fields"))
		return
	}
	var out Cooperation
	e = m.run(r, p, fence.PermNodesContribute, func(tx pgx.Tx) error {
		if e := liveNode(r.Context(), tx, id, "project", true); e != nil {
			return e
		}
		var oldRaw []byte
		var oldRevision int64
		e := tx.QueryRow(r.Context(), `SELECT data,revision FROM crm_project_cooperation WHERE project_node_id=$1::uuid FOR UPDATE`, id).Scan(&oldRaw, &oldRevision)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if oldRevision != in.ExpectedRevision {
			return errConflict
		}
		raw, _ := json.Marshal(in)
		e = tx.QueryRow(r.Context(), `INSERT INTO crm_project_cooperation(tenant_id,project_node_id,data) VALUES($1::uuid,$2::uuid,$3::jsonb) ON CONFLICT(tenant_id,project_node_id) DO UPDATE SET data=excluded.data,revision=crm_project_cooperation.revision+1 RETURNING revision`, p.TenantID, id, raw).Scan(&in.Revision)
		if e != nil {
			return e
		}
		out = in
		out.ExpectedRevision = 0
		return appendCRM(r.Context(), tx, p, id, "crm.project_cooperation_changed", json.RawMessage(oldRaw), out)
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 200, out)
}

func (m *module) draftNote(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, true)
	if !ok {
		return
	}
	id, e := pathUUID(r, "organisationId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var in struct {
		DraftText        string `json:"draft_text"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if e = decodeCRM(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	if strings.TrimSpace(in.DraftText) == "" || len(in.DraftText) > 20000 || in.ExpectedRevision < 1 {
		writeErr(w, errInvalid("invalid note draft"))
		return
	}
	var draftID string
	e = m.run(r, p, fence.PermNodesContribute, func(tx pgx.Tx) error {
		c, e := customer(r.Context(), tx, id, true)
		if e != nil {
			return e
		}
		if c.Revision != in.ExpectedRevision {
			return errConflict
		}
		e = tx.QueryRow(r.Context(), `INSERT INTO crm_note_rewrite_drafts(tenant_id,organisation_node_id,base_revision,proposed_text,proposed_by_principal_id) VALUES($1::uuid,$2::uuid,$3,$4,$5::uuid) RETURNING id::text`, p.TenantID, id, c.Revision, in.DraftText, p.ID).Scan(&draftID)
		if e != nil {
			return e
		}
		return appendCRM(r.Context(), tx, p, id, "crm.note_rewrite_drafted", nil, map[string]any{"draft_id": draftID, "base_revision": c.Revision})
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 201, map[string]any{"id": draftID, "organisation_node_id": id, "draft_text": in.DraftText, "applied": false})
}
func (m *module) applyNote(w http.ResponseWriter, r *http.Request) {
	p, ok := m.actor(w, r, true)
	if !ok {
		return
	}
	org, e := pathUUID(r, "organisationId")
	if e != nil {
		writeErr(w, e)
		return
	}
	id, e := pathUUID(r, "draftId")
	if e != nil {
		writeErr(w, e)
		return
	}
	var out Customer
	e = m.run(r, p, fence.PermNodesContribute, func(tx pgx.Tx) error {
		before, e := customer(r.Context(), tx, org, true)
		if e != nil {
			return e
		}
		var base int64
		var text string
		var applied *time.Time
		e = tx.QueryRow(r.Context(), `SELECT base_revision,proposed_text,applied_at FROM crm_note_rewrite_drafts WHERE id=$1::uuid AND organisation_node_id=$2::uuid FOR UPDATE`, id, org).Scan(&base, &text, &applied)
		if errors.Is(e, pgx.ErrNoRows) {
			return errNotFound
		}
		if e != nil {
			return e
		}
		if applied != nil || base != before.Revision {
			return errConflict
		}
		_, e = tx.Exec(r.Context(), `UPDATE nodes SET fields=jsonb_set(fields,'{customer_notes}',to_jsonb($1::text),true),updated_at=clock_timestamp() WHERE id=$2::uuid`, text, org)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE crm_note_rewrite_drafts SET applied_at=clock_timestamp() WHERE id=$1::uuid`, id)
		if e != nil {
			return e
		}
		out, e = customer(r.Context(), tx, org, false)
		if e != nil {
			return e
		}
		return appendCRM(r.Context(), tx, p, org, "crm.note_rewrite_applied", before, map[string]any{"customer": out, "draft_id": id})
	})
	if e != nil {
		writeErr(w, e)
		return
	}
	httpapi.WriteJSON(w, 200, out)
}
