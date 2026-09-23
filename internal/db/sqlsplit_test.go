// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"sort"
	"strings"
	"testing"
)

func TestSplitSQL(t *testing.T) {
	in := `
-- leading
CREATE TABLE t (id int); -- trailing
/* block
   semi; colon */
CREATE TABLE u (note text);
SELECT 'a;b', 'it''s';
SELECT $$ a; b $$;
SELECT $tag$ a; b $tag$;
`
	got := splitSQL(in)
	want := []string{
		"CREATE TABLE t (id int)",
		"CREATE TABLE u (note text)",
		"SELECT 'a;b', 'it''s'",
		"SELECT $$ a; b $$",
		"SELECT $tag$ a; b $tag$",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d statements:\n%s", len(got), strings.Join(got, "\n---\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stmt %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

func TestSplitCoreMigrations(t *testing.T) {
	names, err := migrationNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no embedded migrations")
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if strings.Join(sorted, "\n") != strings.Join(names, "\n") {
		t.Fatalf("migrations not sorted: %v", names)
	}
	for _, name := range names {
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if len(splitSQL(string(body))) == 0 {
			t.Fatalf("%s has no statements", name)
		}
	}
	body, err := migrationFiles.ReadFile("migrations/0003_principals.sql")
	if err != nil {
		t.Fatal(err)
	}
	stmts := splitSQL(string(body))
	if len(stmts) != 5 {
		t.Fatalf("0003 has %d statements: %q", len(stmts), stmts)
	}
	joined := strings.Join(stmts, "\n")
	for _, needle := range []string{
		"CREATE TABLE principals",
		"CREATE UNIQUE INDEX principals_tenant_identity",
		"ENABLE ROW LEVEL SECURITY",
		"FORCE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation",
		"aeon.tenant_id",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("0003 missing %s", needle)
		}
	}
}
