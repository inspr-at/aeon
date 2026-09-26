// SPDX-License-Identifier: AGPL-3.0-only
package profile

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

// ImportCommand implements paimos import paimos-profiles. The coordinator wires
// this function in cmd/aeon/import.go. It reads classic only through GET,
// previews by default, and changes Aeon only when --apply is provided.
func ImportCommand(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("paimos import paimos-profiles", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourceURL := flags.String("source-url", "", "classic Paimos URL")
	keyFile := flags.String("api-key-file", "", "bearer key file")
	tenantSlug := flags.String("tenant", "", "Aeon tenant slug")
	apply := flags.Bool("apply", false, "write profile changes to Aeon")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *sourceURL == "" || *keyFile == "" || *tenantSlug == "" {
		return errors.New("usage: paimos import paimos-profiles --source-url URL --api-key-file FILE --tenant SLUG [--apply]")
	}
	source, err := NewClassicSource(*sourceURL, *keyFile, nil)
	if err != nil {
		return err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	// A dry run must not apply pending migrations as a side effect.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect target database: %w", err)
	}
	job := ProfileImporter{Pool: pool, Store: attachments.Store{FilesDir: cfg.FilesDir}, Source: source}
	report, err := job.Run(ctx, *tenantSlug, *apply)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(report)
}
