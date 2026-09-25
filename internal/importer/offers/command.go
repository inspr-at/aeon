// SPDX-License-Identifier: AGPL-3.0-only
package offers

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/inspr-at/aeon/internal/config"
	"github.com/inspr-at/aeon/internal/db"
)

// RunCommand is the coordinator's `aeon import paimos-offers` entry point.
// --bundle - consumes a tar stream from stdin. Dry-run is the default and
// needs no target database; --apply requires an explicit tenant and admin.
func RunCommand(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	f := flag.NewFlagSet("aeon import paimos-offers", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	bundlePath := f.String("bundle", "-", "EQ0 export directory or - for tar on stdin")
	instance := f.String("source-instance", "", "stable classic instance name")
	tenantID := f.String("tenant-id", "", "Aeon tenant UUID")
	actorID := f.String("actor-principal-id", "", "Aeon admin principal UUID")
	apply := f.Bool("apply", false, "write the mapped records")
	if err := f.Parse(args); err != nil || f.NArg() != 0 || *instance == "" {
		return errors.New("usage: aeon import paimos-offers --source-instance NAME [--bundle DIR|-] [--tenant-id UUID --actor-principal-id UUID --apply]")
	}
	var b Bundle
	var err error
	if *bundlePath == "-" {
		b, err = ReadTar(stdin)
	} else {
		b, err = ReadDir(*bundlePath)
	}
	if err != nil {
		return fmt.Errorf("read offer bundle: %w", err)
	}
	if !*apply {
		report, err := Import(ctx, nil, "", "", *instance, b, false)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(report)
	}
	if *tenantID == "" || *actorID == "" {
		return errors.New("--apply needs --tenant-id and --actor-principal-id")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer pool.Close()
	report, err := Import(ctx, pool, *tenantID, *actorID, *instance, b, true)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(report)
}
