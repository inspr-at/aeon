// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/jackc/pgx/v5"
)

type finalizeWrite struct {
	ExpectedQuoteRevision  int64  `json:"expected_quote_revision"`
	ExpectedDraftRevision  int64  `json:"expected_draft_revision"`
	ExpectedDocumentSHA256 string `json:"expected_document_sha256"`
}

func documentDigest(id string, version int, offerNo string, doc quoteDocument) (string, error) {
	payload := struct {
		Mode        string        `json:"mode"`
		QuoteNodeID string        `json:"quote_node_id"`
		Version     int           `json:"version"`
		OfferNo     string        `json:"offer_no"`
		Document    quoteDocument `json:"document"`
	}{"document-v1", id, version, offerNo, doc}
	b, e := json.Marshal(payload)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func decimalCents(cents int64) string { return fmt.Sprintf("%d.%02d00", cents/100, cents%100) }
func rateCents(rate decimal, quantity int64) (int64, error) {
	n := new(big.Int).Mul(big.NewInt(int64(rate)), big.NewInt(quantity))
	n.Add(n, big.NewInt(5000))
	n.Div(n, big.NewInt(10000))
	if !n.IsInt64() || n.Int64() > 1_000_000_000_000 {
		return 0, bad("rate total too large")
	}
	return n.Int64(), nil
}
func (m *Module) finalize(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !admin(p) {
		respond(w, 0, nil, denied())
		return
	}
	id, e := pathID(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var in finalizeWrite
	if e = decode(r, &in); e != nil {
		respond(w, 0, nil, e)
		return
	}
	if in.ExpectedQuoteRevision < 1 || in.ExpectedDraftRevision < 1 || !shaRe.MatchString(in.ExpectedDocumentSHA256) {
		respond(w, 0, nil, bad("invalid finalization precondition"))
		return
	}
	var out quote
	e = m.tx(r.Context(), p, fence.PermStepsApply, true, func(tx pgx.Tx) error {
		q, err := readQuote(r.Context(), tx, id, true)
		if err != nil {
			return err
		}
		draft, err := readDraft(r.Context(), tx, id, true)
		if err != nil {
			return err
		}
		if q.Revision != in.ExpectedQuoteRevision || draft.DraftRevision != in.ExpectedDraftRevision || draft.DocumentSHA256 != in.ExpectedDocumentSHA256 {
			return conflict("quote or draft revision is stale")
		}
		if q.State != "draft" || q.Archived || q.OfferNo == "" {
			return conflict("quote cannot be finalized")
		}
		doc, err := decodeDocument(draft.Document)
		if err != nil {
			return err
		}
		if err = validateDocument(&doc, true); err != nil {
			return err
		}
		settings, err := readSettings(r.Context(), tx)
		if err != nil {
			return err
		}
		day, err := quoteDay(settings, time.Now())
		if err != nil {
			return err
		}
		if doc.ValidUntil < day.Format("2006-01-02") {
			return bad("quote validity has expired")
		}
		var customerNo string
		err = tx.QueryRow(r.Context(), `SELECT customer_no FROM crm_customer_numbers WHERE organisation_node_id=$1::uuid`, q.CustomerOrgNodeID).Scan(&customerNo)
		if err != nil {
			return err
		}
		var recipient struct {
			CustomerNo    string `json:"customer_no"`
			ContactNodeID string `json:"contact_node_id"`
		}
		if json.Unmarshal(doc.Recipient, &recipient) != nil || recipient.CustomerNo != customerNo {
			return conflict("customer number changed")
		}
		if recipient.ContactNodeID != "" {
			if !uuidRe.MatchString(recipient.ContactNodeID) {
				return bad("invalid recipient contact")
			}
			var linked bool
			err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM node_relations rel JOIN nodes n ON n.tenant_id=rel.tenant_id AND n.id=rel.source_node_id JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE rel.type='contact_for' AND rel.source_node_id=$1::uuid AND rel.target_node_id=$2::uuid AND n.deleted_at IS NULL AND k.slug='contact')`, recipient.ContactNodeID, q.CustomerOrgNodeID).Scan(&linked)
			if err != nil {
				return err
			}
			if !linked {
				return bad("recipient contact is not linked to customer")
			}
		}
		rateSnapshots := make([]string, len(doc.Positions))
		var subtotal int64
		for i := range doc.Positions {
			line := &doc.Positions[i]
			if line.PricingSource == "cost_unit" {
				var rateText string
				err = tx.QueryRow(r.Context(), `SELECT r.bill_amount::text FROM cost_unit_rates r JOIN nodes n ON n.tenant_id=r.tenant_id AND n.id=r.cost_unit_node_id JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE r.cost_unit_node_id=$1::uuid AND r.currency=$2 AND r.unit=$3 AND r.effective_from<=$4::date AND (r.effective_until IS NULL OR r.effective_until>$4::date) AND n.deleted_at IS NULL AND k.slug='cost_unit' ORDER BY r.effective_from DESC LIMIT 1 FOR SHARE OF r`, line.CostUnitNodeID, doc.Currency, line.RateUnit, doc.OfferDate).Scan(&rateText)
				if errors.Is(err, pgx.ErrNoRows) {
					return bad("no effective cost unit rate")
				}
				if err != nil {
					return err
				}
				rateSnapshots[i] = rateText
				rate, err := parseDecimal(rateText, false)
				if err != nil {
					return err
				}
				qty, err := parseQuantity(line.Quantity)
				if err != nil {
					return err
				}
				line.TotalCents, err = rateCents(rate, qty)
				if err != nil {
					return err
				}
				line.UnitPriceCents, err = rateCents(rate, 100)
				if err != nil {
					return err
				}
			}
			if line.TotalCents > 1_000_000_000_000-subtotal {
				return bad("quote total too large")
			}
			subtotal += line.TotalCents
		}
		doc.NetTotalCents = subtotal
		versionNo := q.CurrentVersion + 1
		digest, err := documentDigest(id, versionNo, q.OfferNo, doc)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		var contact any
		if recipient.ContactNodeID != "" {
			contact = recipient.ContactNodeID
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO quote_versions(tenant_id,quote_node_id,version,recipient_contact_node_id,currency,title,subtotal,tax_total,total,content_sha256,created_by_principal_id,digest_mode,pricing_mode) VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::numeric,0,$7::numeric,$8,$9::uuid,'document-v1','cent-half-up-v1')`, p.TenantID, id, versionNo, contact, doc.Currency, doc.Title, decimalCents(subtotal), digest, p.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO quote_version_snapshots(tenant_id,quote_node_id,version,document,sender,recipient,legal,layout,offer_no,customer_no,project_ref,offer_date,valid_until,validity_time_zone,document_schema_version,renderer_version) VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10,$11,$12::date,$13::date,$14,1,'document-v1')`, p.TenantID, id, versionNo, string(raw), string(doc.Sender), string(doc.Recipient), string(doc.Legal), string(doc.Layout), q.OfferNo, customerNo, doc.ProjectRef, doc.OfferDate, doc.ValidUntil, settings.NumberingTimeZone)
		if err != nil {
			return err
		}
		for i, line := range doc.Positions {
			var costID, rateText any
			if line.PricingSource == "cost_unit" {
				costID = line.CostUnitNodeID
				rateText = rateSnapshots[i]
			}
			_, err = tx.Exec(r.Context(), `INSERT INTO quote_document_lines(tenant_id,quote_node_id,version,position,line_id,pricing_source,short_text,long_text,unit_label,cost_unit_node_id,currency,quantity,unit_price_cents,total_cents,rate_amount,net_amount) VALUES($1::uuid,$2::uuid,$3,$4,$5::uuid,$6,$7,$8,$9,$10::uuid,$11,$12::numeric,$13,$14,$15::numeric,$16::numeric)`, p.TenantID, id, versionNo, i, line.ID, line.PricingSource, line.ShortText, line.LongText, line.UnitLabel, costID, doc.Currency, line.Quantity, line.UnitPriceCents, line.TotalCents, rateText, decimalCents(line.TotalCents))
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(r.Context(), `UPDATE business_quotes SET current_version=$1,revision=revision+1,project_ref=$2 WHERE quote_node_id=$3::uuid`, versionNo, doc.ProjectRef, id)
		if err != nil {
			return err
		}
		if err = appendEvent(r.Context(), tx, p, id, "quote.version_created", map[string]any{"version": q.CurrentVersion}, map[string]any{"version": versionNo, "content_sha256": digest}); err != nil {
			return err
		}
		ev, err := events.Append(r.Context(), tx, p, events.Change{NodeID: &id, Type: "quote.issued", Before: map[string]any{"state": "draft"}, After: map[string]any{"version": versionNo, "content_sha256": digest}})
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO quote_issues(tenant_id,quote_node_id,version,issued_by_principal_id,event_id) VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5)`, p.TenantID, id, versionNo, p.ID, ev.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `UPDATE business_quotes SET state='issued',revision=revision+1 WHERE quote_node_id=$1::uuid`, id)
		if err != nil {
			return err
		}
		out, err = readQuote(r.Context(), tx, id, false)
		return err
	})
	respond(w, 200, out, e)
}

type visibilityWrite struct {
	ExpectedRevision int64 `json:"expected_revision"`
	Archived         bool  `json:"archived"`
}

func (m *Module) visibility(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !admin(p) {
		respond(w, 0, nil, denied())
		return
	}
	id, e := pathID(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var in visibilityWrite
	if e = decode(r, &in); e != nil {
		respond(w, 0, nil, e)
		return
	}
	var out quote
	e = m.tx(r.Context(), p, fence.PermNodesContribute, true, func(tx pgx.Tx) error {
		q, err := readQuote(r.Context(), tx, id, true)
		if err != nil {
			return err
		}
		if q.Revision != in.ExpectedRevision {
			return conflict("quote revision is stale")
		}
		if q.Archived == in.Archived {
			out = q
			return nil
		}
		var at, by any
		if in.Archived {
			at = time.Now()
			by = p.ID
		}
		_, err = tx.Exec(r.Context(), `UPDATE business_quotes SET archived_at=$1::timestamptz,archived_by_principal_id=$2::uuid,revision=revision+1 WHERE quote_node_id=$3::uuid`, at, by, id)
		if err != nil {
			return err
		}
		out, err = readQuote(r.Context(), tx, id, false)
		if err != nil {
			return err
		}
		return appendEvent(r.Context(), tx, p, id, "quote.visibility_changed", map[string]any{"archived": q.Archived}, map[string]any{"archived": out.Archived})
	})
	respond(w, 200, out, e)
}
func cloneDocumentIDs(doc *quoteDocument) error {
	for i := range doc.Sections {
		id, e := newID()
		if e != nil {
			return e
		}
		doc.Sections[i].ID = id
		for j := range doc.Sections[i].Nodes {
			id, e := newID()
			if e != nil {
				return e
			}
			doc.Sections[i].Nodes[j].ID = id
		}
	}
	for i := range doc.Positions {
		id, e := newID()
		if e != nil {
			return e
		}
		doc.Positions[i].ID = id
	}
	return nil
}
func (m *Module) duplicate(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !staff(p) {
		respond(w, 0, nil, denied())
		return
	}
	id, e := pathID(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var in struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if e = decode(r, &in); e != nil {
		respond(w, 0, nil, e)
		return
	}
	var out quote
	e = m.tx(r.Context(), p, fence.PermNodesContribute, true, func(tx pgx.Tx) error {
		source, err := readQuote(r.Context(), tx, id, false)
		if err != nil {
			return err
		}
		settings, err := readSettings(r.Context(), tx)
		if err != nil {
			return err
		}
		day, err := quoteDay(settings, time.Now())
		if err != nil {
			return err
		}
		customerNo, err := ensureCustomerNumber(r.Context(), tx, p, source.CustomerOrgNodeID, day)
		if err != nil {
			return err
		}
		source, err = readQuote(r.Context(), tx, id, true)
		if err != nil {
			return err
		}
		if source.Revision != in.ExpectedRevision {
			return conflict("quote revision is stale")
		}
		var raw []byte
		if source.State != "draft" && source.CurrentVersion > 0 {
			err = tx.QueryRow(r.Context(), `SELECT document FROM quote_version_snapshots WHERE quote_node_id=$1::uuid AND version=$2`, id, source.CurrentVersion).Scan(&raw)
		} else {
			err = tx.QueryRow(r.Context(), `SELECT document FROM quote_drafts WHERE quote_node_id=$1::uuid`, id).Scan(&raw)
		}
		if errors.Is(err, pgx.ErrNoRows) && source.CurrentVersion > 0 {
			err = tx.QueryRow(r.Context(), `SELECT document FROM quote_version_snapshots WHERE quote_node_id=$1::uuid AND version=$2`, id, source.CurrentVersion).Scan(&raw)
		}
		if err != nil {
			return err
		}
		doc, err := decodeDocument(raw)
		if err != nil {
			return err
		}
		if err = cloneDocumentIDs(&doc); err != nil {
			return err
		}
		doc.OfferDate = day.Format("2006-01-02")
		doc.ValidUntil = day.AddDate(0, 0, 30).Format("2006-01-02")
		var recipient map[string]any
		if json.Unmarshal(doc.Recipient, &recipient) != nil {
			return bad("invalid recipient")
		}
		recipient["customer_no"] = customerNo
		currentRecipient, err := customerRecipient(r.Context(), tx, source.CustomerOrgNodeID, customerNo)
		if err != nil {
			return err
		}
		for _, field := range []string{"contact", "email", "contact_node_id"} {
			recipient[field] = currentRecipient[field]
		}
		doc.Recipient, err = json.Marshal(recipient)
		if err != nil {
			return err
		}
		offerNo, err := allocateOfferNumber(r.Context(), tx, p, day)
		if err != nil {
			return err
		}
		if err = tx.QueryRow(r.Context(), `INSERT INTO nodes(tenant_id,key,kind_id,title) SELECT $1::uuid,aeon_next_node_key($1::uuid,k.short_prefix),k.id,$2 FROM node_kinds k WHERE k.tenant_id=$1::uuid AND k.slug='quote' RETURNING id::text`, p.TenantID, doc.Title).Scan(&out.QuoteNodeID); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO business_quotes(tenant_id,quote_node_id,project_node_id,customer_org_node_id,offer_no,project_ref) VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,$4::uuid,$5,$6)`, p.TenantID, out.QuoteNodeID, source.ProjectNodeID, source.CustomerOrgNodeID, offerNo, doc.ProjectRef)
		if err != nil {
			return err
		}
		if err := validateDocument(&doc, false); err != nil {
			return err
		}
		raw, err = json.Marshal(doc)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO quote_drafts(tenant_id,quote_node_id,document,schema_version,minimum_writer_version,updated_by_principal_id) VALUES($1::uuid,$2::uuid,$3::jsonb,1,1,$4::uuid)`, p.TenantID, out.QuoteNodeID, string(raw), p.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1::uuid,$2::uuid,$3::uuid,'customer_of')`, p.TenantID, source.CustomerOrgNodeID, out.QuoteNodeID)
		if err != nil {
			return err
		}
		out, err = readQuote(r.Context(), tx, out.QuoteNodeID, false)
		if err != nil {
			return err
		}
		return appendEvent(r.Context(), tx, p, out.QuoteNodeID, "quote.duplicated", nil, map[string]any{"quote": out, "source_quote_node_id": id})
	})
	respond(w, 201, out, e)
}
func classicStatus(ctx context.Context, tx pgx.Tx, q quote) (string, error) {
	switch q.State {
	case "draft":
		return "draft", nil
	case "accepted":
		return "accepted", nil
	case "void":
		return "void", nil
	case "issued":
		var valid, zone string
		err := tx.QueryRow(ctx, `SELECT valid_until::text,validity_time_zone FROM quote_version_snapshots WHERE quote_node_id=$1::uuid AND version=$2`, q.QuoteNodeID, q.CurrentVersion).Scan(&valid, &zone)
		if errors.Is(err, pgx.ErrNoRows) {
			return "sent", nil
		}
		if err != nil {
			return "", err
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return "", err
		}
		day := time.Now().In(loc)
		if valid < day.Format("2006-01-02") {
			return "expired", nil
		}
		return "sent", nil
	}
	return strings.ToLower(q.State), nil
}
