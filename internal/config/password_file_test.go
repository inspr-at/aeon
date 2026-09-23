// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(f, []byte("s3cr3t\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEON_DATABASE_URL", "postgres://aeon@db:5432/aeon?sslmode=disable")
	t.Setenv("AEON_DATABASE_PASSWORD_FILE", f)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.DatabaseURL, "postgres://aeon:s3cr3t@db:5432/aeon") {
		t.Fatalf("password not applied")
	}
}
