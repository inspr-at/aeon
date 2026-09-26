// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoCommandDispatchAndGuards(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "aeon")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build aeon: %v: %s", err, out)
	}
	for _, tc := range []struct {
		name, env, want string
		args            []string
	}{
		{"non-dev", "prod", "allowed only when AEON_ENV=dev", []string{"demo", "seed", "--tenant", "lumen-demo"}},
		{"missing-tenant", "dev", "usage: aeon demo seed --tenant SLUG", []string{"demo", "seed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(binary, tc.args...)
			cmd.Env = append(os.Environ(), "AEON_ENV="+tc.env)
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "demo: ") || !strings.Contains(string(out), tc.want) {
				t.Fatalf("expected demo guard %q, got err %v, output %s", tc.want, err, out)
			}
		})
	}
}
