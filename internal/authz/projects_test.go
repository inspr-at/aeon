// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/tenant"
)

// ProjectsTx decides from one read of the bindings exactly what RequireTx
// decides with a read per question, for every kind of caller: workspace
// roles, project-only guests, deactivated people, agents with scopes, and a
// key capped by its creator (AEON-184 decides names per project this way).
func TestProjectsTxMatchesRequireTx(t *testing.T) {
	w := newMatrixWorld(t)
	ctx := t.Context()
	for name, p := range w.people {
		if p.Kind == tenant.Agent {
			p.Scopes = []string{"nodes.read", "harness.read", "members.read"}
		}
		t.Run(name, func(t *testing.T) {
			err := db.InTenant(ctx, w.d.App, w.tid, func(tx pgx.Tx) error {
				check, err := ProjectsTx(ctx, tx, p)
				if err != nil {
					return err
				}
				for _, permission := range []string{"nodes.read", "harness.read", "members.read", "nodes.write", "approvals.decide", "no.such"} {
					for _, project := range []string{"", w.projectA, w.projectB} {
						want := RequireTx(ctx, tx, p, permission, Scope{ProjectID: project})
						if want != nil && !errors.Is(want, ErrForbidden) {
							return want
						}
						if got := check(permission, project); got != (want == nil) {
							t.Errorf("%s in %q: ProjectsTx %v, RequireTx %v", permission, project, got, want)
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
