// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"slices"
	"testing"
)

// The only production agent key after P1 carries exactly these permissions.
// The aeon/paimos CLI reads kinds and projects before most commands, so an
// agent with this set must reach its whole heartbeat and inbox path; P1 first
// mapped GET /api/kinds to kinds.read and locked this key out.
func TestCoordinatorPermissionsCoverCLIHeartbeatPath(t *testing.T) {
	have := []string{"harness.read", "harness.worker", "harness.write", "inbox.read", "inbox.send", "nodes.read", "work_orders.read"}
	for _, pattern := range []string{
		"GET /api/kinds",
		"GET /api/projects",
		"POST /api/projects/{projectId}/harness-sessions/{sessionId}/heartbeat",
	} {
		want, ok := PermissionForPattern(pattern)
		if !ok {
			t.Fatalf("%s has no declared permission", pattern)
		}
		if !slices.Contains(have, want) {
			t.Errorf("%s needs %s, which the coordinator key set lacks", pattern, want)
		}
	}
}
