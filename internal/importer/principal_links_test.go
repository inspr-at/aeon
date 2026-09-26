// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"fmt"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/principallink"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
)

func TestClassicUsernameEmailAndCanonicalAssignments(t *testing.T) {
	d := dbtest.Open(t)
	tid, err := tenantbootstrap.Create(t.Context(), d.App, "identity-import", "Identity import")
	if err != nil {
		t.Fatal(err)
	}
	s := Snapshot{SourceID: "source", Users: []Record{{"id": 7, "username": "mba", "full_name": "Markus Barta", "email": "mba@example.test", "role": "member"}}, Projects: []Project{{Record: Record{"id": 1, "key": "ID", "name": "Identity"}, Issues: []Record{{"id": 1, "issue_key": "ID-1", "title": "Ticket", "type": "ticket", "assignee_id": 7}}}}}
	writer := PostgresWriter{Pool: d.App}
	if _, err := writer.Write(t.Context(), s, "identity-import"); err != nil {
		t.Fatal(err)
	}
	var alias string
	txdo := func(fn func(pgx.Tx) error) {
		t.Helper()
		if err := db.InTenant(dbtest.Seed(t.Context()), d.App, tid, fn); err != nil {
			t.Fatal(err)
		}
	}
	txdo(func(tx pgx.Tx) error {
		var email, name, storedEmail string
		if err := tx.QueryRow(t.Context(), `SELECT p.id::text,p.name,p.email,e.after->'classic'->>'email' FROM principals p JOIN events e ON e.tenant_id=p.tenant_id AND e.after->'principal'->>'id'=p.id::text WHERE p.tenant_id=$1 AND e.type='import.user_created'`, tid).Scan(&alias, &name, &email, &storedEmail); err != nil {
			return err
		}
		if name != "mba" || email != "mba@example.test" || email != storedEmail {
			return fmt.Errorf("lost username/email")
		}
		return nil
	})
	target, err := tenantbootstrap.BindOIDC(t.Context(), d.App, "identity-import", "https://id.example.test", "markus", "Markus Barta", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := principallink.New(d.App).Link(t.Context(), "identity-import", alias, target); err != nil {
		t.Fatal(err)
	}
	// Partial user payloads preserve email. Future imports write the canonical
	// assignee while immutable classic records retain source IDs and username.
	delete(s.Users[0], "email")
	if _, err := writer.Write(t.Context(), s, "identity-import"); err != nil {
		t.Fatal(err)
	}
	txdo(func(tx pgx.Tx) error {
		var name, email, assigned string
		if err := tx.QueryRow(t.Context(), `SELECT name,email FROM principals WHERE tenant_id=$1 AND id=$2`, tid, alias).Scan(&name, &email); err != nil {
			return err
		}
		if name != "mba" || email != "mba@example.test" {
			return fmt.Errorf("partial snapshot erased metadata")
		}
		if err := tx.QueryRow(t.Context(), `SELECT fields->>'assignee' FROM nodes WHERE tenant_id=$1 AND key='ID-1'`, tid).Scan(&assigned); err != nil {
			return err
		}
		if assigned != target {
			return fmt.Errorf("import persisted alias %s", assigned)
		}
		return nil
	})
	if report, err := writer.Write(t.Context(), s, "identity-import"); err != nil || report.Updated != 0 {
		t.Fatalf("replay %+v %v", report, err)
	}
	// Repair existing imports locally from the stored identity email, with an
	// event, then prove a repeat creates no additional mutation.
	txdo(func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE principals SET email=NULL WHERE tenant_id=$1 AND id=$2`, tid, alias)
		return err
	})
	if report, err := BackfillPrincipals(t.Context(), d.App, tid); err != nil || report.Counts["emails"] != 1 {
		t.Fatalf("email backfill %+v %v", report, err)
	}
	if report, err := BackfillPrincipals(t.Context(), d.App, tid); err != nil || report.Writes != 0 {
		t.Fatalf("backfill replay %+v %v", report, err)
	}
}
