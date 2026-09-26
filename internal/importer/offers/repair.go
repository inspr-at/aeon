// SPDX-License-Identifier: AGPL-3.0-only
package offers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/authz"
	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/events"
	"github.com/inspr-at/paimos/internal/tenant"
)

type RepairReport struct {
	SourceInstance string `json:"source_instance"`
	Scanned        int    `json:"scanned"`
	Repaired       int    `json:"repaired"`
}

// RepairDraftDimensions changes only numeric prose layout measurements to
// exact decimal strings in imported, currently editable drafts. It preserves
// issued snapshots and every other document field. Each changed draft gets a
// new revision and event; a repeat run makes no changes.
func RepairDraftDimensions(ctx context.Context, pool *pgxpool.Pool, tenantID, actorID, instance string) (RepairReport, error) {
	report := RepairReport{SourceInstance: instance}
	if pool == nil || !uuidRE.MatchString(tenantID) || !uuidRE.MatchString(actorID) || !instanceRE.MatchString(instance) {
		return report, errors.New("repair requires pool, tenant ID, admin principal ID and source instance")
	}
	err := db.InTenant(db.AllProjects(ctx, "classic offers importer"), pool, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":paimos-offers:"+instance); err != nil {
			return err
		}
		actor := tenant.Principal{TenantID: tenantID, ID: actorID, Kind: tenant.Person}
		if err := authz.RequireTx(ctx, tx, actor, "imports.manage", authz.Scope{}); err != nil {
			return errors.New("actor requires import management permission")
		}
		rows, err := tx.Query(ctx, `SELECT d.quote_node_id::text,d.document,d.draft_revision,q.revision
			FROM paimos_offer_imports i
			JOIN quote_drafts d ON d.tenant_id=i.tenant_id AND d.quote_node_id=i.node_id
			JOIN business_quotes q ON q.tenant_id=d.tenant_id AND q.quote_node_id=d.quote_node_id
			WHERE i.tenant_id=$1::uuid AND i.source_instance=$2 AND i.source_kind='offer' AND q.state='draft' AND q.deleted_at IS NULL
			ORDER BY d.quote_node_id FOR UPDATE OF d,q`, tenantID, instance)
		if err != nil {
			return err
		}
		type draft struct {
			id                      string
			document                []byte
			revision, quoteRevision int64
		}
		var drafts []draft
		for rows.Next() {
			var d draft
			if err := rows.Scan(&d.id, &d.document, &d.revision, &d.quoteRevision); err != nil {
				rows.Close()
				return err
			}
			drafts = append(drafts, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, d := range drafts {
			report.Scanned++
			decoder := json.NewDecoder(bytes.NewReader(d.document))
			decoder.UseNumber()
			var document map[string]any
			if err := decoder.Decode(&document); err != nil {
				return fmt.Errorf("draft %s: %w", d.id, err)
			}
			changed := false
			if sections, ok := document["sections"].([]any); ok {
				for i, value := range sections {
					section, ok := value.(map[string]any)
					if !ok {
						continue
					}
					nodes, ok := section["nodes"].([]any)
					if !ok {
						continue
					}
					for j, value := range nodes {
						node, ok := value.(map[string]any)
						if !ok {
							continue
						}
						converted, err := normalizeNodeDimensions(node)
						if err != nil {
							return fmt.Errorf("draft %s section %d node %d: %w", d.id, i, j, err)
						}
						changed = changed || converted
					}
				}
			}
			if !changed {
				continue
			}
			raw, err := json.Marshal(document)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE quote_drafts SET document=$1::jsonb,draft_revision=draft_revision+1,updated_at=clock_timestamp(),updated_by_principal_id=$2::uuid WHERE quote_node_id=$3::uuid`, string(raw), actorID, d.id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE business_quotes SET revision=revision+1 WHERE quote_node_id=$1::uuid`, d.id); err != nil {
				return err
			}
			_, err = events.Append(ctx, tx, actor, events.Change{NodeID: &d.id, Type: "quote.draft_import_repaired", Before: map[string]any{"draft_revision": d.revision, "quote_revision": d.quoteRevision}, After: map[string]any{"draft_revision": d.revision + 1, "quote_revision": d.quoteRevision + 1, "repair": "numeric prose dimensions"}})
			if err != nil {
				return err
			}
			report.Repaired++
		}
		return nil
	})
	return report, err
}
