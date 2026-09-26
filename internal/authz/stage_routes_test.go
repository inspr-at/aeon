// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"strings"
	"testing"
)

// Routed plugin agents (pharos: deploy/verify, janus: prepare/apply) post
// evidence and results with their stage.<op> scope; the handler then checks
// the routed principal and the live grant. Requiring stage_handoffs.write or
// .decide at the route would lock those keys out before the handler runs.
func TestStageHandoffWritesAcceptRoutedOpScopes(t *testing.T) {
	for _, pattern := range []string{
		"POST /api/stage-handoffs/{handoffId}/evidence",
		"POST /api/stage-handoffs/{handoffId}/result",
		"GET /api/stage-handoffs/{handoffId}",
	} {
		perm, ok := PermissionForPattern(pattern)
		if !ok {
			t.Fatalf("%s has no declared permission", pattern)
		}
		for _, op := range []string{"stage.prepare", "stage.deploy", "stage.verify", "stage.apply"} {
			if !slicesContains(strings.Split(perm, "|"), op) {
				t.Errorf("%s = %q, lacks %s", pattern, perm, op)
			}
		}
	}
}

func slicesContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
