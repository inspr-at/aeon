// SPDX-License-Identifier: AGPL-3.0-only

package imports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
)

func TestImportStatusTenantPagination(t *testing.T) {
	db := dbtest.Open(t)
	ctx := t.Context()
	var tenantID, principalID, otherTenantID, otherPrincipalID string
	if err := db.Admin.QueryRow(ctx, `INSERT INTO tenants (slug, name) VALUES ('imports-test', 'Imports') RETURNING id::text`).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	if err := db.Admin.QueryRow(ctx, `INSERT INTO principals (tenant_id, kind, name) VALUES ($1, 'person', 'caller') RETURNING id::text`, tenantID).Scan(&principalID); err != nil {
		t.Fatal(err)
	}
	if err := db.Admin.QueryRow(ctx, `INSERT INTO tenants (slug, name) VALUES ('imports-other', 'Other') RETURNING id::text`).Scan(&otherTenantID); err != nil {
		t.Fatal(err)
	}
	if err := db.Admin.QueryRow(ctx, `INSERT INTO principals (tenant_id, kind, name) VALUES ($1, 'person', 'other') RETURNING id::text`, otherTenantID).Scan(&otherPrincipalID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Microsecond)
	var olderID, newerID, hiddenID string
	for _, job := range []struct {
		tenantID, principalID, source string
		createdAt                     time.Time
		id                            *string
	}{
		{tenantID, principalID, "older.csv", base, &olderID},
		{tenantID, principalID, "newer.csv", base.Add(time.Second), &newerID},
		{otherTenantID, otherPrincipalID, "hidden.csv", base.Add(2 * time.Second), &hiddenID},
	} {
		if err := db.Admin.QueryRow(ctx, `INSERT INTO import_jobs (tenant_id, created_by_principal_id, source, created_at) VALUES ($1, $2, $3, $4) RETURNING id::text`, job.tenantID, job.principalID, job.source, job.createdAt).Scan(job.id); err != nil {
			t.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	New(db.App).Mount(mux)
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(tenant.WithPrincipal(req.Context(), tenant.Principal{ID: principalID, TenantID: tenantID}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	first := request("/api/imports?limit=1")
	if first.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", first.Code, first.Body.String())
	}
	var page struct {
		Items      []importJob `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != newerID || page.NextCursor == nil {
		t.Fatalf("first page = %#v", page)
	}
	second := request("/api/imports?limit=1&cursor=" + *page.NextCursor)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", second.Code, second.Body.String())
	}
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != olderID || page.NextCursor != nil {
		t.Fatalf("second page = %#v", page)
	}
	foreign := request("/api/imports/" + hiddenID)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant import status=%d body=%s", foreign.Code, foreign.Body.String())
	}
}
