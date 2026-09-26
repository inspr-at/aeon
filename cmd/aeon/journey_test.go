// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestJourneyOperatorDispatchGuardsBothCommands(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "aeon")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build aeon: %v: %s", err, out)
	}
	for _, command := range []string{"mark-disposable", "seed"} {
		t.Run(command, func(t *testing.T) {
			base := []string{"journey", command, "--tenant", "example", "--project", "PRJ-1"}
			if command == "seed" {
				base = append(base, "--to-stage", "build")
			}
			for _, tc := range []struct {
				name  string
				flags []string
				want  string
			}{
				{"missing flags", nil, "requires --production and --confirm-project"},
				{"missing confirmation", []string{"--production"}, "requires --production and --confirm-project"},
				{"mismatched confirmation", []string{"--production", "--confirm-project", "PRJ-2"}, "must exactly match --project"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cmd := exec.Command(binary, append(append([]string{}, base...), tc.flags...)...)
					cmd.Env = append(os.Environ(), "AEON_ENV=prod")
					out, err := cmd.CombinedOutput()
					if err == nil || !strings.Contains(string(out), tc.want) {
						t.Fatalf("expected host operator guard, got err %v, output %s", err, out)
					}
				})
			}
		})
	}
}
