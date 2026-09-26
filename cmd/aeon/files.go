// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/attachments"
	"github.com/inspr-at/paimos/internal/config"
)

// filesCommand runs the operator-only attachment store checks:
//
//	paimos files verify --tenant SLUG
//	paimos files gc --tenant SLUG [--apply]
func filesCommand(args []string, stdout io.Writer) error {
	const usage = "usage: paimos files verify --tenant SLUG | paimos files gc --tenant SLUG [--apply]"
	if len(args) == 0 || (args[0] != "verify" && args[0] != "gc") {
		return errors.New(usage)
	}
	flags := flag.NewFlagSet("paimos files "+args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tenant := flags.String("tenant", "", "tenant slug")
	apply := flags.Bool("apply", false, "gc only: delete instead of listing")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *tenant == "" {
		return errors.New(usage)
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	store := attachments.Store{FilesDir: cfg.FilesDir}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		var tenantID string
		if err := pool.QueryRow(ctx, `SELECT id FROM tenants WHERE slug=$1`, *tenant).Scan(&tenantID); err != nil {
			return fmt.Errorf("tenant %q: %w", *tenant, err)
		}
		var report any
		if args[0] == "verify" {
			report, err = attachments.Verify(ctx, pool, store, tenantID)
		} else {
			report, err = attachments.GC(ctx, pool, store, tenantID, *apply)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(report)
	})
}
