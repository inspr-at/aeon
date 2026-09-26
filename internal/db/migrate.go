// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/config"
	"github.com/inspr-at/paimos/internal/linkvault"
)

// migrationLock is the session advisory-lock key held for one migration run.
const migrationLock int64 = 780002

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrate applies each unrecorded SQL file in its own transaction.
// A later call skips files already listed in schema_migrations.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return MigrateWithHook(ctx, pool, nil)
}

// MigrateWithHook runs pending migrations and calls before just before each
// file. Migration tests use it to populate an older schema; production passes
// no hook through migrate.
func MigrateWithHook(ctx context.Context, pool *pgxpool.Pool, before func(string) error) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLock); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLock)
	}()

	// pgvector is created by the deployment as superuser. Migrations only
	// ensure the extension is present.
	if _, err := conn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("vector extension: %w", err)
	}
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	for _, name := range names {
		if applied[name] {
			continue
		}
		if before != nil {
			if err := before(name); err != nil {
				return fmt.Errorf("before %s: %w", name, err)
			}
		}
		if name == "0771_quote_link_drop_plaintext.sql" {
			if err := migratePublicLinkTokens(ctx, pool); err != nil {
				return fmt.Errorf("migrate public link tokens: %w", err)
			}
		}
		if err := applyFile(ctx, conn, name); err != nil {
			return err
		}
	}
	return nil
}

// migratePublicLinkTokens runs after 0770 and before the plaintext column is
// dropped. Each tenant is scoped independently. A missing host key discards
// only the re-copy vault; existing SHA-256 verifiers remain active. Retrying
// after an interrupted migration is safe because converted rows are skipped.
func migratePublicLinkTokens(ctx context.Context, pool *pgxpool.Pool) error {
	key, err := config.LinkKeyFromEnv()
	if err != nil {
		return err
	}
	var tenantIDs []string
	err = InTenant(ctx, pool, "00000000-0000-0000-0000-000000000000", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text FROM tenants ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			tenantIDs = append(tenantIDs, id)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, tenantID := range tenantIDs {
		err = InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
			if key == nil {
				_, err := tx.Exec(ctx, `DELETE FROM quote_public_link_tokens WHERE tenant_id=$1::uuid`, tenantID)
				return err
			}
			rows, err := tx.Query(ctx, `SELECT link_id::text,token FROM quote_public_link_tokens WHERE tenant_id=$1::uuid AND ciphertext IS NULL`, tenantID)
			if err != nil {
				return err
			}
			type oldToken struct{ id, token string }
			var old []oldToken
			for rows.Next() {
				var item oldToken
				if err := rows.Scan(&item.id, &item.token); err != nil {
					rows.Close()
					return err
				}
				old = append(old, item)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			for _, item := range old {
				ciphertext, err := linkvault.Encrypt(key, tenantID, item.id, item.token)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE quote_public_link_tokens SET ciphertext=$1 WHERE tenant_id=$2::uuid AND link_id=$3::uuid`, ciphertext, tenantID, item.id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func applyFile(ctx context.Context, conn *pgxpool.Conn, name string) error {
	body, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	stmts := splitSQL(string(body))
	if len(stmts) == 0 {
		return fmt.Errorf("migration %s has no statements", name)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}
