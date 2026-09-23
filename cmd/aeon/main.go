// SPDX-License-Identifier: AGPL-3.0-only

// Command aeon is the PAIMOS AEON binary. `aeon serve` runs the server.
// The same binary answers the agent command line, and behaves as paimos when
// argv[0] is paimos.
package main

import (
	"fmt"
	"os"

	"github.com/inspr-at/aeon/internal/cli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(cli.Run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
