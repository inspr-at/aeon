// SPDX-License-Identifier: AGPL-3.0-only
package plugins_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/janus"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestTenantAdminInstallationWritesEventAndFencesPermission(t *testing.T) {
	ctx := context.Background()
	fresh := dbtest.Open(t)
	var tenantID string
	if err := fresh.Admin.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('plugin-test','Plugin test') RETURNING id::text`).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	p := tenant.Principal{TenantID: tenantID, Kind: tenant.Person, Roles: []string{"admin"}}
	err := db.InTenant(ctx, fresh.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person','Admin',ARRAY['admin']) RETURNING id::text`, tenantID).Scan(&p.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	r := plugins.NewRegistry()
	if err := janus.Register(r); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	plugins.New(fresh.App, r).Mount(mux)
	man := janus.Manifest()
	body, _ := json.Marshal(map[string]any{"manifest_digest_sha256": man.DigestSHA256, "enabled": true, "permissions": []string{"prepare"}})
	request := httptest.NewRequest(http.MethodPut, "/api/plugins/janus/installation", bytes.NewReader(body))
	request = request.WithContext(tenant.WithPrincipal(request.Context(), p))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != 200 {
		t.Fatalf("install status=%d body=%s", w.Code, w.Body.String())
	}
	err = db.InTenant(ctx, fresh.App, tenantID, func(tx pgx.Tx) error {
		enabled, err := plugins.Enabled(ctx, tx, r, "janus", "prepare")
		if err != nil {
			return err
		}
		if !enabled {
			t.Fatal("prepare not enabled")
		}
		enabled, err = plugins.Enabled(ctx, tx, r, "janus", "apply")
		if err != nil {
			return err
		}
		if enabled {
			t.Fatal("apply escaped permission subset")
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM plugin_installation_events`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("installation events=%d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	bad := bytes.Replace(body, []byte(man.DigestSHA256), []byte("0000000000000000000000000000000000000000000000000000000000000000"), 1)
	request = httptest.NewRequest(http.MethodPut, "/api/plugins/janus/installation", bytes.NewReader(bad))
	request = request.WithContext(tenant.WithPrincipal(request.Context(), p))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != 409 {
		t.Fatalf("digest mismatch status=%d", w.Code)
	}
}
func TestRegistryRejectsSchemaCollisionAndUndeclaredCapability(t *testing.T) {
	r := plugins.NewRegistry()
	first := plugins.Seal(plugins.Manifest{ID: "one", Version: "1", Owner: "test", Permissions: []string{"read"}, NodeKinds: []plugins.NodeKind{{Slug: "custom", FieldSchema: json.RawMessage(`{"type":"object"}`)}}})
	if err := r.Register(first); err != nil {
		t.Fatal(err)
	}
	second := plugins.Seal(plugins.Manifest{ID: "two", Version: "1", Owner: "test", NodeKinds: []plugins.NodeKind{{Slug: "custom", FieldSchema: json.RawMessage(`{"type":"object"}`)}}})
	if err := r.Register(second); err == nil {
		t.Fatal("schema collision accepted")
	}
	third := plugins.Seal(plugins.Manifest{ID: "three", Version: "1", Owner: "test", AgentTools: []plugins.Capability{{ID: "tool", Permission: "undeclared"}}})
	if err := r.Register(third); err == nil {
		t.Fatal("undeclared permission accepted")
	}
}
