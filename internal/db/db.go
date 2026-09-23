// SPDX-License-Identifier: AGPL-3.0-only

// Package db owns the Postgres pool, migrations and tenant-scoped transactions.
// Shared contract between P0.2 (implements) and P0.3 (uses); extend, do not rename.
package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantSetting is the Postgres setting row-level security policies read.
const TenantSetting = "aeon.tenant_id"

// ErrNotImplemented marks stubs P0.2 replaces.
var ErrNotImplemented = errors.New("not implemented")

// Open connects the pool and runs pending migrations. P0.2 implements.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) { return nil, ErrNotImplemented }

// InTenant runs fn in one transaction with TenantSetting set to tenantID
// (set_config(..., true)), so every RLS policy applies. P0.2 implements.
func InTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	return ErrNotImplemented
}
