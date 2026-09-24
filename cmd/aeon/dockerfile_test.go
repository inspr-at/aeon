// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"strings"
	"testing"
)

// The runtime UID/GID is a contract with the host: csb1's aeon-files
// directory is owned by 65532. v260924170915 lost file access when the image
// user floated to an alpine default (UID 100).
func TestDockerfileRuntimeUserIs65532(t *testing.T) {
	data, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\nUSER 65532:65532\n") {
		t.Fatal("Dockerfile must run as USER 65532:65532")
	}
}
