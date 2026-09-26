// SPDX-License-Identifier: AGPL-3.0-only
package principallink

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run handles arguments following `paimos principal`. The coordinator supplies
// the operator pool and owns configuration, migration and process exit status.
func Run(ctx context.Context, pool *pgxpool.Pool, args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "link" && args[0] != "unlink") {
		return errors.New("usage: paimos principal link|unlink --tenant SLUG --from ID_OR_NAME [--to ID_OR_NAME] [--suggest]")
	}
	fs := flag.NewFlagSet("principal "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	slug := fs.String("tenant", "", "tenant slug")
	from := fs.String("from", "", "source principal ID or name")
	to := fs.String("to", "", "target principal ID or name")
	suggest := fs.Bool("suggest", false, "list likely pairs without changes")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || *slug == "" {
		return errors.New("tenant is required; unexpected positional arguments are forbidden")
	}
	service := New(pool)
	if *suggest {
		if args[0] != "link" || *from != "" || *to != "" {
			return errors.New("--suggest requires link and cannot be combined with --from or --to")
		}
		pairs, err := service.Suggest(ctx, *slug)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(pairs)
	}
	if *from == "" || (args[0] == "unlink" && *to != "") {
		return errors.New("--from is required; unlink does not accept --to")
	}
	var result Result
	var err error
	if args[0] == "link" {
		result, err = service.Link(ctx, *slug, *from, *to)
	} else {
		result, err = service.Unlink(ctx, *slug, *from)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
