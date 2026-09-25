// SPDX-License-Identifier: AGPL-3.0-only

// Command aeon is the PAIMOS AEON binary. `aeon serve` runs the server.
// The same binary answers the agent command line, and behaves as paimos when
// argv[0] is paimos.
package main

import (
	"context"
	"fmt"
	"os"
	// Embed the IANA time zone database: the runtime image (alpine) has no
	// /usr/share/zoneinfo, and time zones drive greetings, profiles, hours and
	// quote numbering.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/cli"
	"github.com/inspr-at/aeon/internal/principallink"
	"github.com/inspr-at/aeon/internal/profile"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "import" && os.Args[2] == "paimos" {
		if err := importPaimos(os.Args[3:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "import:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "import" && os.Args[2] == "paimos-profiles" {
		if err := profile.ImportCommand(context.Background(), os.Args[3:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "import:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "import" && os.Args[2] == "paimos-attachments" {
		if err := importPaimosAttachments(os.Args[3:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "import:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "import" && os.Args[2] == "backfill-relations" {
		if err := backfillRelations(os.Args[3:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "import:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "agent-key" {
		if err := agentKeyCommand(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "agent-key:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "principal" {
		if err := withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
			return principallink.Run(ctx, pool, os.Args[2:], os.Stdout)
		}); err != nil {
			fmt.Fprintln(os.Stderr, "principal:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "files" {
		if err := filesCommand(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "files:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "tenant" {
		if err := tenantCommand(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "tenant:", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(cli.RunMessaging(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
