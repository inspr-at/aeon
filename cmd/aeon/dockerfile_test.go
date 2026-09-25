// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"strings"
	"testing"
)

// The runtime UID/GID is a contract with the host: csb1's aeon-files
// directory is owned by 65532. v260924170915 lost file access when the image
// user floated to an alpine default (UID 100). The last USER in the runtime
// stage is the one the image runs as; an earlier 65532 line does not count.
func TestDockerfileRuntimeUserIs65532(t *testing.T) {
	data, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	stages := strings.Split("\n"+string(data), "\nFROM ")
	runtime := stages[len(stages)-1]
	var lastUser string
	for _, line := range strings.Split(runtime, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "USER ") {
			lastUser = strings.TrimSpace(strings.TrimPrefix(line, "USER "))
		}
	}
	if lastUser != "65532:65532" {
		t.Fatalf("runtime USER %q, want 65532:65532", lastUser)
	}
	if !strings.Contains(runtime, "adduser -S -D -u 65532 ") {
		t.Fatal("runtime stage must create uid 65532")
	}
}
