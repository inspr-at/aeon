// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"slices"
	"testing"
)

func TestPrincipalContextPreservesLegacyLabels(t *testing.T) {
	for _, tc := range []struct {
		kind  PrincipalKind
		roles []string
	}{
		{Person, []string{"super_admin"}},
		{Person, []string{"admin"}},
		{Agent, []string{"admin", "super_admin"}},
		{Person, []string{"member"}},
	} {
		p, ok := PrincipalFrom(WithPrincipal(context.Background(), Principal{Kind: tc.kind, Roles: tc.roles}))
		if !ok || !slices.Equal(p.Roles, tc.roles) {
			t.Errorf("kind %s roles %v became %v", tc.kind, tc.roles, p.Roles)
		}
	}
}
