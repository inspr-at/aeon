// SPDX-License-Identifier: AGPL-3.0-only

package auth

import "testing"

// Existing agent keys carry dot scopes (nodes.read); SEC1's core-route check
// briefly required colon scopes and locked every agent out of projects and
// nodes (2026-09-25). Both separators must keep matching.
func TestHasScopeAcceptsDotAndColon(t *testing.T) {
	cases := []struct {
		have []string
		want string
		ok   bool
	}{
		{[]string{"nodes.read"}, "nodes.read", true},
		{[]string{"nodes.read"}, "nodes:read", true},
		{[]string{"nodes:read"}, "nodes.read", true},
		{[]string{"harness.read", "inbox.send"}, "nodes.read", false},
		{[]string{"nodes.read"}, "nodes.write", false},
	}
	for _, c := range cases {
		if got := hasScope(c.have, c.want); got != c.ok {
			t.Errorf("hasScope(%v, %q) = %v, want %v", c.have, c.want, got, c.ok)
		}
	}
}
