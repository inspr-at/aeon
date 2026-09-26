// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/attachments"
	"github.com/inspr-at/aeon/internal/config"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/importer"
)

func importPaimos(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("aeon import paimos", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourceURL := flags.String("source-url", "", "classic Paimos URL")
	keyFile := flags.String("api-key-file", "", "bearer key file")
	tenant := flags.String("tenant", "", "Aeon tenant slug")
	project := flags.String("project", "", "classic project key")
	dryRun := flags.Bool("dry-run", false, "read and report without writing")
	concurrency := flags.Int("concurrency", 4, "maximum concurrent source requests")
	delay := flags.Duration("delay", 0, "minimum delay between source request starts")
	delta := flags.Bool("delta", false, "import only items that are new or changed since the last import")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid import flags")
	}
	if flags.NArg() != 0 || *sourceURL == "" || *keyFile == "" || *tenant == "" {
		return errors.New("usage: aeon import paimos --source-url URL --api-key-file FILE --tenant SLUG [--project KEY] [--dry-run] [--delta] [--concurrency N] [--delay DURATION]")
	}
	if *delta && *dryRun {
		return errors.New("--delta writes; use aeon import reconcile to preview differences")
	}
	source, err := importer.NewHTTPSource(*sourceURL, *keyFile, nil)
	if err != nil {
		return err
	}
	if err := source.Configure(*concurrency, *delay); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	job := importer.Importer{Source: source}
	if !*dryRun {
		cfg, err := config.FromEnv()
		if err != nil {
			return err
		}
		pool, err := db.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("open target database: %w", err)
		}
		defer pool.Close()
		job.Writer = importer.PostgresWriter{Pool: pool}
	}
	if *delta {
		report, err := job.RunDelta(ctx, *tenant, *project)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(report)
	}
	report, err := job.Run(ctx, *tenant, *project, *dryRun)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(report)
}

// backfillRelations replays stored import.relation events into node relations
// and release membership. It never contacts the classic source.
func backfillRelations(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("aeon import backfill-relations", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tenant := flags.String("tenant", "", "Aeon tenant slug")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *tenant == "" {
		return errors.New("usage: aeon import backfill-relations --tenant SLUG")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer pool.Close()
	var tenantID string
	if err := pool.QueryRow(ctx, `SELECT id FROM tenants WHERE slug=$1`, *tenant).Scan(&tenantID); err != nil {
		return fmt.Errorf("tenant %q: %w", *tenant, err)
	}
	report, err := importer.BackfillRelations(ctx, pool, tenantID)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(report)
}

// importPaimosAttachments downloads the files of attachment records in a
// classic Paimos snapshot and attaches them to the already imported nodes.
// It reads the classic instance only; nodes and their fields are not touched.
func importPaimosAttachments(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("aeon import paimos-attachments", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourceURL := flags.String("source-url", "", "classic Paimos URL")
	keyFile := flags.String("api-key-file", "", "bearer key file")
	tenant := flags.String("tenant", "", "Aeon tenant slug")
	project := flags.String("project", "", "classic project key (default: all)")
	concurrency := flags.Int("concurrency", 4, "maximum concurrent source requests")
	delay := flags.Duration("delay", 0, "minimum delay between source request starts")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *sourceURL == "" || *keyFile == "" || *tenant == "" {
		return errors.New("usage: aeon import paimos-attachments --source-url URL --api-key-file FILE --tenant SLUG [--project KEY] [--concurrency N] [--delay DURATION]")
	}
	source, err := importer.NewHTTPSource(*sourceURL, *keyFile, nil)
	if err != nil {
		return err
	}
	if err := source.Configure(*concurrency, *delay); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer pool.Close()
	var tenantID, actorID string
	if err := pool.QueryRow(ctx, `SELECT id FROM tenants WHERE slug=$1`, *tenant).Scan(&tenantID); err != nil {
		return fmt.Errorf("tenant %q: %w", *tenant, err)
	}
	if err := db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM principals WHERE tenant_id=$1 AND kind='agent' AND name='Classic Paimos importer' ORDER BY created_at LIMIT 1`, tenantID).Scan(&actorID)
	}); err != nil {
		return fmt.Errorf("importer principal (run aeon import paimos first): %w", err)
	}
	snap, err := source.Read(ctx, *project)
	if err != nil {
		return err
	}
	created, err := importer.ImportAttachments(ctx, pool, attachments.Store{FilesDir: cfg.FilesDir}, source, snap, tenantID, actorID)
	if err != nil {
		return fmt.Errorf("after %d attachments: %w", created, err)
	}
	return json.NewEncoder(stdout).Encode(map[string]int{"attachments_created": created})
}

// importReconcile compares the classic source with the Aeon tenant import. It
// only reads: GET against classic, SELECT against Aeon.
func importReconcile(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("aeon import reconcile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourceURL := flags.String("source-url", "", "classic Paimos URL")
	keyFile := flags.String("api-key-file", "", "bearer key file")
	tenant := flags.String("tenant", "", "Aeon tenant slug")
	project := flags.String("project", "", "classic project key (all projects when empty)")
	// Reconcile reads the whole source like an import does; one request at a
	// time takes hours against a production-sized classic (cutover AEON-43).
	concurrency := flags.Int("concurrency", 4, "maximum concurrent source requests")
	delay := flags.Duration("delay", 0, "minimum delay between source request starts")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *sourceURL == "" || *keyFile == "" || *tenant == "" {
		return errors.New("usage: aeon import reconcile --source-url URL --api-key-file FILE --tenant SLUG [--project KEY] [--concurrency N] [--delay DURATION]")
	}
	source, err := importer.NewHTTPSource(*sourceURL, *keyFile, nil)
	if err != nil {
		return err
	}
	if err := source.Configure(*concurrency, *delay); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer pool.Close()
	report, err := importer.Reconcile(ctx, source, pool, attachments.Store{FilesDir: cfg.FilesDir}, *tenant, *project)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(report)
}
