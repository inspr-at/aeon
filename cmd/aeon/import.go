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
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid import flags")
	}
	if flags.NArg() != 0 || *sourceURL == "" || *keyFile == "" || *tenant == "" {
		return errors.New("usage: aeon import paimos --source-url URL --api-key-file FILE --tenant SLUG [--project KEY] [--dry-run] [--concurrency N] [--delay DURATION]")
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
	report, err := job.Run(ctx, *tenant, *project, *dryRun)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(report)
}
