// SPDX-License-Identifier: AGPL-3.0-only

// Command aeon is the single PAIMOS AEON binary: `aeon serve` runs the server.
package main

import (
	"fmt"
	"os"

	"github.com/inspr-at/aeon/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: aeon <serve|version>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version.Version)
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
