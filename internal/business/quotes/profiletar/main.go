// SPDX-License-Identifier: AGPL-3.0-only

// Command profiletar packs a checked-in document profile bundle directory into
// the reproducible tar that `aeon quote-profile apply --bundle -` reads from
// stdin, and prints the tar's SHA-256:
//
//	go run ./internal/business/quotes/profiletar -src internal/business/quotes/profiles/inspr -out dist/quote-profile-inspr.tar
//
// The directory is read with the same checks the apply command uses (no
// symlinks, no paths outside the root, size limits). Files the profile does
// not reference, such as font licences and provenance, travel in the tar too.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/inspr-at/paimos/internal/business/quotes"
)

func main() {
	src := flag.String("src", "", "bundle directory with profile.json")
	out := flag.String("out", "", "tar file to write (- for stdout)")
	flag.Parse()
	if *src == "" || *out == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: profiletar -src DIR -out FILE|-")
		os.Exit(2)
	}
	if err := run(*src, *out); err != nil {
		fmt.Fprintln(os.Stderr, "profiletar:", err)
		os.Exit(1)
	}
}

func run(src, out string) error {
	bundle, err := quotes.ReadProfileBundleDir(src)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := quotes.WriteProfileBundleTar(&buf, bundle); err != nil {
		return err
	}
	sum := sha256.Sum256(buf.Bytes())
	if out == "-" {
		_, err = os.Stdout.Write(buf.Bytes())
		fmt.Fprintf(os.Stderr, "%s  -\n", hex.EncodeToString(sum[:]))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s  %s\n", hex.EncodeToString(sum[:]), out)
	return nil
}
