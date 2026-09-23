// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type installRow struct {
	pluginID string
	version  string
	digest   string
	owner    string
	enabled  bool
	perms    []string
	updated  time.Time
}

func (row installRow) installation() Installation {
	return Installation{
		ManifestDigestSHA256: row.digest,
		Enabled:              row.enabled,
		Permissions:          cloneStrings(row.perms),
		PluginID:             row.pluginID,
		Version:              row.version,
		UpdatedAt:            row.updated,
	}
}

func loadInstall(ctx context.Context, tx pgx.Tx, tenantID, pluginID string, lock bool) (*installRow, error) {
	query := `SELECT plugin_id, version, manifest_digest_sha256, owner, enabled, permissions, updated_at
		FROM plugin_installations
		WHERE tenant_id = $1::uuid AND plugin_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var row installRow
	err := tx.QueryRow(ctx, query, tenantID, pluginID).Scan(
		&row.pluginID, &row.version, &row.digest, &row.owner, &row.enabled, &row.perms, &row.updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.perms = cloneStrings(row.perms)
	slices.Sort(row.perms)
	return &row, nil
}

func listInstalls(ctx context.Context, tx pgx.Tx, tenantID string) (map[string]installRow, error) {
	rows, err := tx.Query(ctx, `SELECT plugin_id, version, manifest_digest_sha256, owner, enabled, permissions, updated_at
		FROM plugin_installations
		WHERE tenant_id = $1::uuid
		ORDER BY plugin_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]installRow{}
	for rows.Next() {
		var row installRow
		if err := rows.Scan(&row.pluginID, &row.version, &row.digest, &row.owner, &row.enabled, &row.perms, &row.updated); err != nil {
			return nil, err
		}
		row.perms = cloneStrings(row.perms)
		slices.Sort(row.perms)
		out[row.pluginID] = row
	}
	return out, rows.Err()
}
