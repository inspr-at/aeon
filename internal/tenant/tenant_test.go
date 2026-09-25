// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"slices"
	"testing"
)

func TestGlobalAdminNormalizationIsPersonOnly(t *testing.T) {
	for _, tc := range []struct {
		kind  PrincipalKind
		roles []string
		admin bool
	}{
		{Person, []string{"super_admin"}, true},
		{Person, []string{"admin"}, true},
		{Agent, []string{"admin", "super_admin"}, false},
		{Person, []string{"member"}, false},
	} {
		p, ok := PrincipalFrom(WithPrincipal(context.Background(), Principal{Kind: tc.kind, Roles: tc.roles}))
		if !ok || IsAdmin(p) != tc.admin || (tc.kind == Person && slices.Contains(p.Roles, "admin") != tc.admin) || (tc.kind == Agent && !slices.Equal(p.Roles, tc.roles)) {
			t.Errorf("kind %s roles %v became %v", tc.kind, tc.roles, p.Roles)
		}
	}
}
