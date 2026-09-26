// SPDX-License-Identifier: AGPL-3.0-only
package principallink

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
)

type fixture struct {
	d   *dbtest.DB
	s   *Service
	tid string
	t   *testing.T
}

func setup(t *testing.T) *fixture {
	d := dbtest.Open(t)
	tid, err := tenantbootstrap.Create(t.Context(), d.App, "links", "Links")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{d: d, s: New(d.App), tid: tid, t: t}
}
func (f *fixture) tx(fn func(pgx.Tx) error) {
	f.t.Helper()
	if err := db.InTenant(dbtest.Seed(f.t.Context()), f.d.App, f.tid, fn); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) person(name, issuer, email string) string {
	f.t.Helper()
	var id string
	f.tx(func(tx pgx.Tx) error {
		var identity *string
		if issuer != "" {
			var v string
			if err := tx.QueryRow(f.t.Context(), `INSERT INTO identities(issuer,subject,email) VALUES($1,$2,$3) RETURNING id::text`, issuer, name, email).Scan(&v); err != nil {
				return err
			}
			identity = &v
		}
		return tx.QueryRow(f.t.Context(), `INSERT INTO principals(tenant_id,kind,name,identity_id) VALUES($1,'person',$2,$3) RETURNING id::text`, f.tid, name, identity).Scan(&id)
	})
	return id
}
func (f *fixture) count() int {
	var n int
	f.tx(func(tx pgx.Tx) error {
		return tx.QueryRow(f.t.Context(), `SELECT count(*) FROM events WHERE tenant_id=$1`, f.tid).Scan(&n)
	})
	return n
}
func TestLinkUnlinkReplayAndSuggestions(t *testing.T) {
	f := setup(t)
	a := f.person("mba", "paimos-classic", "same@example.test")
	b := f.person("Markus Barta", "https://id.example.test", "SAME@example.test")
	f.person("same", "paimos-classic", "")
	f.person("unrelated", "paimos-classic", "")
	before := f.count()
	var out bytes.Buffer
	if err := Run(t.Context(), f.d.App, []string{"link", "--tenant", "links", "--suggest"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "same_email") || !strings.Contains(out.String(), "username_email_local_part") || strings.Contains(out.String(), "unrelated") {
		t.Fatal(out.String())
	}
	if f.count() != before {
		t.Fatal("suggest wrote events")
	}
	r, err := f.s.Link(t.Context(), "links", "mba", b)
	if err != nil || !r.Changed || r.Person.LinkedTo == nil || *r.Person.LinkedTo != b {
		t.Fatalf("link %+v %v", r, err)
	}
	n := f.count()
	if r, err = f.s.Link(t.Context(), "links", a, "Markus Barta"); err != nil || r.Changed || f.count() != n {
		t.Fatalf("replay %+v %v", r, err)
	}
	f.tx(func(tx pgx.Tx) error {
		id, name, err := Resolve(t.Context(), tx, f.tid, a)
		if err != nil {
			return err
		}
		if id != b || name != "Markus Barta" {
			return fmt.Errorf("resolution %s %s", id, name)
		}
		var typ, actor, previous string
		if err := tx.QueryRow(t.Context(), `SELECT e.type,p.name,e.before->>'name' FROM events e JOIN principals p ON p.id=e.actor_principal_id AND p.tenant_id=e.tenant_id WHERE e.tenant_id=$1 AND e.type='principal.linked'`, f.tid).Scan(&typ, &actor, &previous); err != nil {
			return err
		}
		if actor != "Principal link operator" || previous != "mba" {
			return fmt.Errorf("wrong audit actor/snapshot")
		}
		return nil
	})
	if r, err = f.s.Unlink(t.Context(), "links", "mba"); err != nil || !r.Changed || r.Person.LinkedTo != nil {
		t.Fatalf("unlink %+v %v", r, err)
	}
	n = f.count()
	if r, err = f.s.Unlink(t.Context(), "links", a); err != nil || r.Changed || f.count() != n {
		t.Fatalf("unlink replay %+v %v", r, err)
	}
	f.tx(func(tx pgx.Tx) error {
		id, name, err := Resolve(t.Context(), tx, f.tid, a)
		if err == nil && (id != a || name != "mba") {
			return fmt.Errorf("unlink resolution")
		}
		return err
	})
}
func TestInvalidLinksAndDatabaseConstraints(t *testing.T) {
	f := setup(t)
	a := f.person("alias", "", "")
	b := f.person("real", "", "")
	c := f.person("third", "", "")
	foreign, err := tenantbootstrap.Create(t.Context(), f.d.App, "foreign", "Foreign")
	if err != nil {
		t.Fatal(err)
	}
	var outside, agent string
	if err := db.InTenant(dbtest.Seed(t.Context()), f.d.App, foreign, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'person','foreign') RETURNING id::text`, foreign).Scan(&outside)
	}); err != nil {
		t.Fatal(err)
	}
	f.tx(func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'agent','bot') RETURNING id::text`, f.tid).Scan(&agent)
	})
	for _, pair := range [][2]string{{a, a}, {a, outside}, {a, agent}, {agent, b}, {"missing", b}} {
		if _, err := f.s.Link(t.Context(), "links", pair[0], pair[1]); err == nil {
			t.Fatalf("accepted %v", pair)
		}
	}
	if _, err := f.s.Link(t.Context(), "links", a, b); err != nil {
		t.Fatal(err)
	}
	n := f.count()
	for _, pair := range [][2]string{{b, a}, {b, c}, {c, a}, {a, c}} {
		if _, err := f.s.Link(t.Context(), "links", pair[0], pair[1]); err == nil {
			t.Fatalf("accepted chain/relink %v", pair)
		}
	}
	if f.count() != n {
		t.Fatal("rejected changes wrote events")
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE principals SET linked_to=$2 WHERE id=$1`, []any{b, c}},
		{`UPDATE principals SET linked_to=$2 WHERE id=$1`, []any{c, a}},
		{`UPDATE principals SET linked_to=$2 WHERE id=$1`, []any{c, outside}},
		{`UPDATE principals SET linked_to=$2 WHERE id=$1`, []any{c, agent}},
		{`UPDATE principals SET linked_to=$2 WHERE id=$1`, []any{agent, c}},
		{`UPDATE principals SET kind='agent' WHERE id=$1`, []any{b}},
	} {
		if err := db.InTenant(dbtest.Seed(t.Context()), f.d.App, f.tid, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), q.sql, q.args...); return err }); err == nil {
			t.Fatal("database accepted invalid topology")
		}
	}
	f.person("duplicate", "", "")
	f.person("duplicate", "", "")
	if _, err := f.s.Link(t.Context(), "links", "duplicate", c); err == nil {
		t.Fatal("ambiguous name accepted")
	}
}
func TestAtomicRollbackAndConcurrentLinks(t *testing.T) {
	f := setup(t)
	a := f.person("a", "", "")
	b := f.person("b", "", "")
	c := f.person("c", "", "")
	// Force event insertion failure after the link update; the update must roll back.
	if err := db.InTenant(dbtest.Seed(t.Context()), f.d.Admin, f.tid, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `CREATE FUNCTION reject_link_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.type='principal.linked' THEN RAISE EXCEPTION 'test failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_link_event BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION reject_link_event()`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Link(t.Context(), "links", a, b); err == nil {
		t.Fatal("expected event failure")
	}
	f.tx(func(tx pgx.Tx) error {
		var link *string
		if err := tx.QueryRow(t.Context(), `SELECT linked_to::text FROM principals WHERE id=$1`, a).Scan(&link); err != nil {
			return err
		}
		if link != nil {
			return fmt.Errorf("link escaped rollback")
		}
		_, err := tx.Exec(t.Context(), `DROP TRIGGER reject_link_event ON events`)
		return err
	})
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, pair := range [][2]string{{a, b}, {b, c}} {
		wg.Add(1)
		go func(pair [2]string) {
			defer wg.Done()
			<-start
			_, err := f.s.Link(context.Background(), "links", pair[0], pair[1])
			results <- err
		}(pair)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent chain successes %d", success)
	}
}
func TestCLIValidation(t *testing.T) {
	for _, args := range [][]string{{}, {"unknown"}, {"link", "--suggest"}, {"link", "--tenant", "links", "--suggest", "--from", "x"}, {"unlink", "--tenant", "links", "--from", "a", "--to", "b"}, {"link", "--tenant", "links", "--from", "a"}, {"link", "--tenant", "links", "extra"}} {
		if err := Run(t.Context(), nil, args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
