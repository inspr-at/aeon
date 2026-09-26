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
			cmd := exec.Command(binary, "journey", command, "--tenant", "example", "--project", "PRJ-1")
			cmd.Env = append(os.Environ(), "AEON_ENV=prod")
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "journey: journey operator commands require AEON_ENV=dev") {
				t.Fatalf("expected host operator guard, got err %v, output %s", err, out)
			}
		})
	}
}
