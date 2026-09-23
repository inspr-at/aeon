// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureTenant inserts the bootstrap tenant when its slug is missing.
// An existing slug is left unchanged.
func EnsureTenant(ctx context.Context, pool *pgxpool.Pool, slug, name string) error {
	if slug == "" || name == "" {
		return errors.New("tenant slug and name are required")
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (slug, name) VALUES ($1, $2)
		ON CONFLICT (slug) DO NOTHING`, slug, name)
	if err != nil {
		return fmt.Errorf("bootstrap tenant: %w", err)
	}
	return nil
}
