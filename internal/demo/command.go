// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/db"
)

// Summary is the JSON `aeon demo seed` prints. It never includes a key.
type Summary struct {
	Slug     string `json:"slug"`
	TenantID string `json:"tenant_id"`
	Already  bool   `json:"already"`
	Projects int    `json:"projects"`
	Tickets  int    `json:"tickets"`
	Stage    string `json:"stage,omitempty"`
}

// Run executes `demo seed --tenant SLUG`.
func Run(ctx context.Context, pool *pgxpool.Pool, args []string, stdout io.Writer) error {
	slug, err := Validate(args)
	if err != nil {
		return err
	}
	sum, err := Seed(ctx, pool, slug)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(sum)
}

// Validate checks the development guard and explicit target before the CLI
// opens a database connection.
func Validate(args []string) (string, error) {
	if err := requireDev(); err != nil {
		return "", err
	}
	if len(args) == 0 || args[0] != "seed" {
		return "", errors.New("usage: aeon demo seed --tenant SLUG")
	}
	fs := flag.NewFlagSet("demo seed", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	slug := fs.String("tenant", "", "tenant slug")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *slug == "" {
		return "", errors.New("usage: aeon demo seed --tenant SLUG")
	}
	return *slug, nil
}

func requireDev() error {
	if os.Getenv("AEON_ENV") != "dev" {
		return errors.New("aeon demo seed is allowed only when AEON_ENV=dev")
	}
	return nil
}

// Seed fills slug. A second call on a completed tenant does not write again.
func Seed(ctx context.Context, pool *pgxpool.Pool, slug string) (Summary, error) {
	return seedWithHook(ctx, pool, slug, nil)
}

func seedWithHook(ctx context.Context, pool *pgxpool.Pool, slug string, afterStep func(string) error) (Summary, error) {
	if err := requireDev(); err != nil {
		return Summary{}, err
	}
	if pool == nil {
		return Summary{}, fmt.Errorf("database pool is required")
	}
	if slug == "" {
		return Summary{}, errors.New("explicit tenant slug is required")
	}
	s := &seeder{pool: pool, slug: slug, afterStep: afterStep}
	if err := db.InTransaction(ctx, pool, func(txCtx context.Context) error {
		s.ctx = txCtx
		return s.run()
	}); err != nil {
		return Summary{}, err
	}
	return s.out, nil
}
