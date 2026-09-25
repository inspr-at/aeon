// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Agent keys reach shared project groups (AEON-136) through the views scope,
// like saved views and preferences.
func TestCoreAgentScopeCoversProjectGroups(t *testing.T) {
	for _, tc := range []struct{ method, path, want string }{
		{http.MethodGet, "/api/project-groups", "views.read"},
		{http.MethodPost, "/api/project-groups", "views.write"},
		{http.MethodPost, "/api/project-groups/assign", "views.write"},
		{http.MethodPatch, "/api/project-groups/7d0d6f36-6f55-4b43-9f42-2a4d7a8a0a11", "views.write"},
		{http.MethodDelete, "/api/project-groups/7d0d6f36-6f55-4b43-9f42-2a4d7a8a0a11", "views.write"},
		{http.MethodGet, "/api/preferences/projects", "views.read"},
	} {
		got, controlled := coreAgentScope(httptest.NewRequest(tc.method, tc.path, nil))
		if !controlled || got != tc.want {
			t.Fatalf("%s %s = %q (controlled %v), want %q", tc.method, tc.path, got, controlled, tc.want)
		}
	}
}
