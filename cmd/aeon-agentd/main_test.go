// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateRegistryFileModes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "accounts.json")
	if err := os.WriteFile(path, []byte(`{"accounts":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := privateFile(path, 1024); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := privateFile(path, 1024); err == nil {
		t.Fatal("world-readable registry accepted")
	}
}

func TestUnknownCommandFailsClosed(t *testing.T) {
	if err := run([]string{"unknown"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown daemon command accepted")
	}
}
