// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"errors"
	"time"

	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// versionSummary is the current version's frozen header without its lines.
type versionSummary struct {
	Version                int       `json:"version"`
	Title                  string    `json:"title"`
	Currency               string    `json:"currency"`
	RecipientContactNodeID string    `json:"recipient_contact_node_id"`
	Subtotal               decimal   `json:"subtotal"`
	TaxTotal               decimal   `json:"tax_total"`
	Total                  decimal   `json:"total"`
	ContentSHA256          string    `json:"content_sha256"`
	LineCount              int       `json:"line_count"`
	CreatedAt              time.Time `json:"created_at"`
}

// quoteView is what list, get, create and issue return: the projection plus
// the R1 node's key and title and a summary of the current version. It is a
// read model only. viewer_can_accept is a display hint for the caller; the
// accept handler checks the binding again inside its own transaction.
type quoteView struct {
	quote
	Key             string          `json:"key"`
	Title           string          `json:"title"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Current         *versionSummary `json:"current"`
	IssuedAt        *time.Time      `json:"issued_at"`
	AcceptedAt      *time.Time      `json:"accepted_at"`
	ViewerCanAccept bool            `json:"viewer_can_accept"`
}

// issueRecord and acceptanceRecord describe the decisions on one version.
type issueRecord struct {
	IssuedByPrincipalID string    `json:"issued_by_principal_id"`
	IssuedAt            time.Time `json:"issued_at"`
	EventID             int64     `json:"event_id"`
}
type acceptanceRecord struct {
	CustomerPrincipalID   string    `json:"customer_principal_id"`
	AcceptedContentSHA256 string    `json:"accepted_content_sha256"`
	AcceptedAt            time.Time `json:"accepted_at"`
	EventID               int64     `json:"event_id"`
}

const viewColumns = `q.quote_node_id::text,q.project_node_id::text,q.customer_org_node_id::text,q.current_version,q.state,q.revision,
 n.key,n.title,q.created_at,
 GREATEST(n.updated_at,q.created_at,COALESCE(v.created_at,q.created_at),COALESCE(i.issued_at,q.created_at),COALESCE(a.accepted_at,q.created_at)),
 v.version,v.title,v.currency,v.recipient_contact_node_id::text,v.subtotal::text,v.tax_total::text,v.total::text,v.content_sha256,v.created_at,
 (SELECT count(*) FROM quote_line_items l WHERE l.tenant_id=q.tenant_id AND l.quote_node_id=q.quote_node_id AND l.version=q.current_version),
 i.issued_at,a.accepted_at,
 (q.state='issued' AND v.version IS NOT NULL AND $1::uuid IS NOT NULL AND EXISTS(
   SELECT 1 FROM crm_contact_principals cp
   JOIN principals pr ON pr.tenant_id=cp.tenant_id AND pr.id=cp.principal_id
   JOIN nodes c ON c.tenant_id=cp.tenant_id AND c.id=cp.contact_node_id
   JOIN node_relations rel ON rel.tenant_id=cp.tenant_id AND rel.source_node_id=cp.contact_node_id AND rel.target_node_id=q.customer_org_node_id AND rel.type='contact_for'
   WHERE cp.contact_node_id=v.recipient_contact_node_id AND cp.principal_id=$1::uuid AND pr.kind='person' AND c.deleted_at IS NULL))`

const viewFrom = ` FROM business_quotes q
 JOIN nodes n ON n.tenant_id=q.tenant_id AND n.id=q.quote_node_id
 LEFT JOIN quote_versions v ON v.tenant_id=q.tenant_id AND v.quote_node_id=q.quote_node_id AND v.version=q.current_version
 LEFT JOIN quote_issues i ON i.tenant_id=q.tenant_id AND i.quote_node_id=q.quote_node_id AND i.version=q.current_version
 LEFT JOIN quote_acceptances a ON a.tenant_id=q.tenant_id AND a.quote_node_id=q.quote_node_id AND a.version=q.current_version`

// viewer is the caller's person ID for the acceptance hint, or nil.
func viewer(p tenant.Principal) any {
	if p.Kind != tenant.Person {
		return nil
	}
	return p.ID
}

func scanView(row pgx.Row) (quoteView, error) {
	var out quoteView
	var version, lines *int
	var title, currency, recipient, subtotal, tax, total, digest *string
	var versionAt *time.Time
	err := row.Scan(&out.QuoteNodeID, &out.ProjectNodeID, &out.CustomerOrgNodeID, &out.CurrentVersion, &out.State, &out.Revision,
		&out.Key, &out.Title, &out.CreatedAt, &out.UpdatedAt,
		&version, &title, &currency, &recipient, &subtotal, &tax, &total, &digest, &versionAt, &lines,
		&out.IssuedAt, &out.AcceptedAt, &out.ViewerCanAccept)
	if err != nil {
		return out, err
	}
	if version != nil {
		s := versionSummary{Version: *version, Title: *title, Currency: *currency, RecipientContactNodeID: *recipient, ContentSHA256: *digest, CreatedAt: *versionAt}
		if lines != nil {
			s.LineCount = *lines
		}
		for _, pair := range []struct {
			text *string
			into *decimal
		}{{subtotal, &s.Subtotal}, {tax, &s.TaxTotal}, {total, &s.Total}} {
			if *pair.into, err = parseDecimal(*pair.text, false); err != nil {
				return out, err
			}
		}
		out.Current = &s
	}
	return out, nil
}

func readView(ctx context.Context, tx pgx.Tx, p tenant.Principal, id string) (quoteView, error) {
	out, err := scanView(tx.QueryRow(ctx, `SELECT `+viewColumns+viewFrom+` WHERE q.quote_node_id=$2::uuid`, viewer(p), id))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, missing()
	}
	return out, err
}

// listViews returns live quotes, newest first. A deleted quote node leaves the
// list; its frozen versions stay readable by ID.
func listViews(ctx context.Context, tx pgx.Tx, p tenant.Principal, project, org string) ([]quoteView, error) {
	rows, err := tx.Query(ctx, `SELECT `+viewColumns+viewFrom+`
 WHERE n.deleted_at IS NULL AND ($2::text='' OR q.project_node_id=NULLIF($2,'')::uuid) AND ($3::text='' OR q.customer_org_node_id=NULLIF($3,'')::uuid)
 ORDER BY q.created_at DESC,q.quote_node_id LIMIT 200`, viewer(p), project, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []quoteView{}
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// attachDecisions adds the issue and acceptance records of one version. Only
// the read endpoints call it; digests and events never include them.
func attachDecisions(ctx context.Context, tx pgx.Tx, v *version) error {
	var issue issueRecord
	err := tx.QueryRow(ctx, `SELECT issued_by_principal_id::text,issued_at,event_id FROM quote_issues WHERE quote_node_id=$1::uuid AND version=$2`, v.QuoteNodeID, v.Version).Scan(&issue.IssuedByPrincipalID, &issue.IssuedAt, &issue.EventID)
	if err == nil {
		v.Issue = &issue
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var acceptance acceptanceRecord
	err = tx.QueryRow(ctx, `SELECT customer_principal_id::text,accepted_content_sha256,accepted_at,event_id FROM quote_acceptances WHERE quote_node_id=$1::uuid AND version=$2`, v.QuoteNodeID, v.Version).Scan(&acceptance.CustomerPrincipalID, &acceptance.AcceptedContentSHA256, &acceptance.AcceptedAt, &acceptance.EventID)
	if err == nil {
		v.Acceptance = &acceptance
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}
