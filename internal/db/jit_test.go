// SPDX-License-Identifier: AGPL-3.0-only

package db_test

import (
	"context"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
)

// Aeon's sessions run without JIT compilation (it cost ~310 ms per tree query on
// production data), unless the database URL asks for it explicitly.
func TestOpenTurnsJITOffUnlessTheURLSetsIt(t *testing.T) {
	ctx := context.Background()
	fresh := dbtest.Open(t)
	for _, tc := range []struct{ suffix, want string }{
		{"", "off"},
		{"&options=-c%20jit%3Don", "on"},
	} {
		pool, err := db.Open(ctx, fresh.URL+tc.suffix)
		if err != nil {
			t.Fatal(err)
		}
		var jit string
		if err := pool.QueryRow(ctx, `SHOW jit`).Scan(&jit); err != nil {
			t.Fatal(err)
		}
		pool.Close()
		if jit != tc.want {
			t.Fatalf("URL%s: jit=%s, want %s", tc.suffix, jit, tc.want)
		}
	}
}
