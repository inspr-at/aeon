// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/jackc/pgx/v5"
)

type branchWrite struct {
	ExpectedQuoteRevision int64  `json:"expected_quote_revision"`
	ExpectedVersion       int    `json:"expected_version"`
	ExpectedContentSHA256 string `json:"expected_content_sha256"`
}

// branchDraft makes a new editable aggregate from a sealed document version.
// The source version and its issue/decision evidence remain unchanged.
func (m *Module) branchDraft(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !m.allow(r, p) {
		respond(w, 0, nil, denied())
		return
	}
	id, e := pathID(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var in branchWrite
	if e = decode(r, &in); e != nil {
		respond(w, 0, nil, e)
		return
	}
	if in.ExpectedQuoteRevision < 1 || in.ExpectedVersion < 1 || !shaRe.MatchString(in.ExpectedContentSHA256) {
		respond(w, 0, nil, bad("invalid branch precondition"))
		return
	}
	var out draftRow
	e = m.tx(r.Context(), p, fence.PermStepsApply, true, func(tx pgx.Tx) error {
		q, err := readQuote(r.Context(), tx, id, true)
		if err != nil {
			return err
		}
		if q.Revision != in.ExpectedQuoteRevision || q.CurrentVersion != in.ExpectedVersion || q.Archived || (q.State != "issued" && q.State != "accepted") {
			return conflict("quote cannot branch from this version")
		}
		v, err := readVersion(r.Context(), tx, id, in.ExpectedVersion)
		if err != nil {
			return err
		}
		if v.DigestMode != "document-v1" || v.ContentSHA256 != in.ExpectedContentSHA256 {
			return conflict("source document digest is stale")
		}
		doc, err := decodeDocument(v.Document)
		if err != nil {
			return err
		}
		sum, err := documentDigest(id, v.Version, v.OfferNo, doc)
		if err != nil {
			return err
		}
		if sum != v.ContentSHA256 {
			return conflict("source document digest mismatch")
		}
		doc.MinimumWriterVersion = documentMinimumWriterVersion(doc)
		_, err = tx.Exec(r.Context(), `UPDATE business_quotes SET state='draft',revision=revision+1 WHERE quote_node_id=$1::uuid`, id)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO quote_drafts(tenant_id,quote_node_id,document,schema_version,minimum_writer_version,base_version,updated_by_principal_id) VALUES($1::uuid,$2::uuid,$3::jsonb,1,$4,$5,$6::uuid) ON CONFLICT(tenant_id,quote_node_id) DO UPDATE SET document=excluded.document,schema_version=excluded.schema_version,minimum_writer_version=excluded.minimum_writer_version,base_version=excluded.base_version,draft_revision=quote_drafts.draft_revision+1,updated_at=clock_timestamp(),updated_by_principal_id=excluded.updated_by_principal_id`, p.TenantID, id, string(raw), doc.MinimumWriterVersion, v.Version, p.ID)
		if err != nil {
			return err
		}
		out, err = readDraft(r.Context(), tx, id, false)
		if err != nil {
			return err
		}
		return appendEvent(r.Context(), tx, p, id, "quote.draft_branched", map[string]any{"quote_revision": q.Revision, "version": v.Version}, map[string]any{"quote_revision": out.QuoteRevision, "draft_revision": out.DraftRevision, "base_version": v.Version})
	})
	if e == nil {
		w.Header().Set("ETag", fmt.Sprintf(`"qd-%d"`, out.DraftRevision))
	}
	respond(w, 200, out, e)
}
