// SPDX-License-Identifier: AGPL-3.0-only

package activity

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestReadOnlyImportedDataset is an opt-in local acceptance probe. It never
// migrates or seeds the target and forces every transaction to be read-only.
// Set AEON_ACTIVITY_PROBE_DATABASE_URL and AEON_ACTIVITY_PROBE_TENANT_ID.
func TestReadOnlyImportedDataset(t *testing.T) {
	raw := os.Getenv("AEON_ACTIVITY_PROBE_DATABASE_URL")
	if raw == "" {
		t.Skip("opt-in read-only imported dataset probe")
	}
	tenantID := os.Getenv("AEON_ACTIVITY_PROBE_TENANT_ID")
	if tenantID == "" {
		t.Fatal("AEON_ACTIVITY_PROBE_TENANT_ID required")
	}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal("invalid probe database URL")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	p := tenant.Principal{TenantID: tenantID, Kind: tenant.Person}
	nodes := map[string]string{}
	err = db.InTenant(t.Context(), pool, tenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT id::text,name FROM principals WHERE tenant_id=$1 AND kind='person' ORDER BY id LIMIT 1`, tenantID).Scan(&p.ID, &p.Name); err != nil {
			return err
		}
		var comments, history int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE type='import.comment'),count(*) FILTER (WHERE type='import.history') FROM events WHERE tenant_id=$1`, tenantID).Scan(&comments, &history); err != nil {
			return err
		}
		t.Logf("read-only dataset: %d imported comments, %d history rows", comments, history)
		rows, err := tx.Query(t.Context(), `SELECT key,id::text FROM nodes WHERE tenant_id=$1 AND key=ANY($2::text[])`, tenantID, []string{"PHAROS-296", "PHAROS-290", "PAI-1048"})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, id string
			if err := rows.Scan(&key, &id); err != nil {
				return err
			}
			nodes[key] = id
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected three realistic tickets, found %d", len(nodes))
	}
	mux := http.NewServeMux()
	New(pool).Mount(mux)
	for key, id := range nodes {
		durations := []time.Duration{}
		for i := 0; i < 20; i++ {
			start := time.Now()
			w := request(mux, &p, "GET", "/api/nodes/"+id+"/activity?limit=50", "")
			elapsed := time.Since(start)
			if w.Code != 200 {
				t.Fatalf("%s status %d: %s", key, w.Code, w.Body.String())
			}
			durations = append(durations, elapsed)
			var page Page
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Items) == 0 {
				t.Fatal("empty imported activity")
			}
			if i == 0 {
				counts := map[string]int{}
				for _, item := range page.Items {
					counts[item.Type]++
					if item.ID == "" || item.At.IsZero() || item.Author.Name == "" {
						t.Fatalf("invalid shape for %s", key)
					}
					if item.Type == "comment" && item.BodyMarkdown == nil {
						t.Fatal("missing comment body")
					}
					if item.Type == "change" && len(item.Changes) == 0 {
						t.Fatal("empty change")
					}
				}
				t.Logf("%s shape: %v; next_cursor=%v", key, counts, page.NextCursor != nil)
			}
		}
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		t.Logf("%s handler latency over 20 requests: median=%s p95=%s max=%s", key, durations[10], durations[18], durations[19])
		if durations[19] >= 100*time.Millisecond {
			t.Errorf("%s exceeds 100ms: %s", key, durations[19])
		}
	}
}
