// SPDX-License-Identifier: AGPL-3.0-only

// Package perf contains the opt-in AEON-130 production-scale local probe.
package perf

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/activity"
	"github.com/inspr-at/aeon/internal/agentaccounts"
	"github.com/inspr-at/aeon/internal/agentruns"
	"github.com/inspr-at/aeon/internal/approvals"
	"github.com/inspr-at/aeon/internal/business/costunits"
	"github.com/inspr-at/aeon/internal/business/crm"
	"github.com/inspr-at/aeon/internal/business/quotes"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/harness"
	"github.com/inspr-at/aeon/internal/nodes"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/search"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// AEON_PERF=1 runs this against a disposable dbtest database. It deliberately
// stays out of the normal test gate because seeding 6,300 rows takes seconds.
func TestLocalEndpointsAtScale(t *testing.T) {
	if os.Getenv("AEON_PERF") != "1" {
		t.Skip("set AEON_PERF=1 for the production-scale local probe")
	}
	ctx := t.Context()
	d := dbtest.Open(t)
	tid := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	reg := plugins.NewRegistry()
	for _, makePlugin := range []func() (plugins.Plugin, error){costunits.Plugin, crm.Plugin, quotes.ManifestPlugin} {
		plug, err := makePlugin()
		if err != nil {
			t.Fatal(err)
		}
		if err := reg.Register(plug); err != nil {
			t.Fatal(err)
		}
	}
	reg.Seal()
	var principal, agent, project, ticket string
	err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1,'perf1','Performance')`, tid); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1,'person','Perf admin',ARRAY['admin']) RETURNING id::text`, tid).Scan(&principal); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'agent','Perf agent') RETURNING id::text`, tid).Scan(&agent); err != nil {
			return err
		}
		for _, id := range []string{costunits.PluginID, crm.ID, quotes.PluginID} {
			plug, _ := reg.Lookup(id)
			if _, err := tx.Exec(ctx, `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1,$2,$3,$4,$5,true,$6,$7)`, tid, id, plug.Manifest.Version, plug.Manifest.DigestSHA256, plug.Manifest.Owner, plug.Manifest.Permissions, principal); err != nil {
				return err
			}
		}
		for _, pair := range [][2]string{{"organisation", "ORG"}, {"quote", "QUO"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1,$2,$2,$3,$2) ON CONFLICT (tenant_id,slug) DO NOTHING`, tid, pair[0], pair[1]); err != nil {
				return err
			}
		}
		var projectKind, ticketKind, orgKind, quoteKind string
		for _, kind := range []struct {
			slug string
			dest *string
		}{{"project", &projectKind}, {"ticket", &ticketKind}, {"organisation", &orgKind}, {"quote", &quoteKind}} {
			if err := tx.QueryRow(ctx, `SELECT id::text FROM node_kinds WHERE tenant_id=$1 AND slug=$2`, tid, kind.slug).Scan(kind.dest); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,state,position) SELECT $1,'PRJ-'||g,$2,'Project '||g,'active',g FROM generate_series(1,35) g`, tid, projectKind); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE tenant_id=$1 AND key='PRJ-1'`, tid).Scan(&project); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `WITH projects AS (SELECT id,row_number() OVER (ORDER BY key) AS rn FROM nodes WHERE tenant_id=$1 AND kind_id=$3)
			INSERT INTO nodes(tenant_id,key,kind_id,title,state,parent_id,position,fields)
			SELECT $1,'TKT-'||g,$2,'Ticket '||g,CASE g%4 WHEN 0 THEN 'new' WHEN 1 THEN 'in_progress' WHEN 2 THEN 'qa' ELSE 'done' END,
			p.id,g,jsonb_build_object('priority',CASE g%3 WHEN 0 THEN 'high' WHEN 1 THEN 'medium' ELSE 'low' END)
			FROM generate_series(1,6065) g JOIN projects p ON p.rn=CASE WHEN g<=4000 THEN 1 ELSE 2+(g-4001)%34 END`, tid, ticketKind, projectKind); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE tenant_id=$1 AND key='TKT-1'`, tid).Scan(&ticket); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,state) SELECT $1,'ORG-'||g,$2,'Customer '||g,'active' FROM generate_series(1,100) g`, tid, orgKind); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,state) SELECT $1,'QUO-'||g,$2,'Quote '||g,'draft' FROM generate_series(1,100) g`, tid, quoteKind); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `WITH q AS (SELECT id,row_number() OVER (ORDER BY key) rn FROM nodes WHERE tenant_id=$1 AND kind_id=$2), o AS (SELECT id,row_number() OVER (ORDER BY key) rn FROM nodes WHERE tenant_id=$1 AND kind_id=$3)
			INSERT INTO business_quotes(tenant_id,quote_node_id,project_node_id,customer_org_node_id) SELECT $1,q.id,$4,o.id FROM q JOIN o USING (rn)`, tid, quoteKind, orgKind, project); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `WITH tickets AS (SELECT id,row_number() OVER (ORDER BY key) rn FROM nodes WHERE tenant_id=$1 AND kind_id=$2)
			INSERT INTO attachments(tenant_id,node_id,sha256,name,content_type,size,created_by)
			SELECT $1,id,md5(rn::text)||md5(rn::text),'image.png','image/png',184320,$3 FROM tickets WHERE rn<=400`, tid, ticketKind, principal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agent_accounts(tenant_id,account_key,harness,daemon_id,registered_by_principal_id,label)
			SELECT $1,'account-'||g,'codex','perf-daemon',$2,'Account '||g FROM generate_series(1,10) g`, tid, agent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO harness_sessions(tenant_id,project_id,agent_principal_id,harness,host,management,role,ref_digest,lease_digest)
			SELECT $1,$2,$3,'codex','perf-host','managed','worker',decode(md5(g::text),'hex'),decode(md5((g+100)::text),'hex') FROM generate_series(1,35) g`, tid, project, agent); err != nil {
			return err
		}
		// Seed events alongside fixture mutations so activity and event indexes see
		// the same order of magnitude as node and attachment tables.
		if _, err := tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after)
			SELECT $1,$2,id,'node.created',jsonb_build_object('id',id) FROM nodes WHERE tenant_id=$1`, tid, principal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after)
			SELECT $1,$2,node_id,'attachment.created',jsonb_build_object('id',id) FROM attachments WHERE tenant_id=$1`, tid, principal); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"nodes", "attachments", "events"} {
		var n int
		if err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error { return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n) }); err != nil {
			t.Fatal(err)
		}
		t.Logf("fixture %s=%d", table, n)
	}
	quoteModule, err := quotes.New(d.App, reg)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for _, mod := range []interface{ Mount(*http.ServeMux) }{
		nodes.New(d.App, nil), search.New(d.App, nil), activity.New(d.App),
		crm.New(d.App, reg), quoteModule, approvals.New(d.App), agentaccounts.New(d.App),
		harness.New(d.App), agentruns.New(d.App),
	} {
		mod.Mount(mux)
	}
	p := tenant.Principal{ID: principal, TenantID: tid, Kind: tenant.Person, Roles: []string{"admin"}}
	for _, target := range []struct{ name, path string }{
		{"projects", "/api/projects"},
		{"nodes_facets", "/api/nodes?within=" + project + "&sort=state,-updated_at&limit=50&facets=state,kind,priority,assignee"},
		{"search", "/api/search?q=Ticket"},
		{"activity", "/api/nodes/" + ticket + "/activity?limit=50"},
		{"quotes", "/api/quotes"},
		{"customers", "/api/crm/organisations?limit=50"},
		{"approvals", "/api/approvals"},
		{"agent_accounts", "/api/agent-accounts"},
		{"harness_sessions", "/api/harness-sessions"},
		{"agent_runs", "/api/runs"},
	} {
		for i := 0; i < 3; i++ {
			call(t, mux, p, target.path)
		}
		samples := make([]time.Duration, 25)
		for i := range samples {
			start := time.Now()
			call(t, mux, p, target.path)
			samples[i] = time.Since(start)
		}
		slices.Sort(samples)
		t.Logf("API %-18s p50=%s p95=%s", target.name, samples[12], samples[23])
	}
	for _, plan := range []struct {
		name, sql string
		args      []any
	}{
		{"customer_index", `SELECT n.id FROM nodes n LEFT JOIN crm_organisation_profiles o ON o.tenant_id=n.tenant_id AND o.organisation_node_id=n.id WHERE n.tenant_id=current_setting('aeon.tenant_id')::uuid AND n.kind_id=(SELECT id FROM node_kinds WHERE tenant_id=current_setting('aeon.tenant_id')::uuid AND slug='organisation') AND n.deleted_at IS NULL AND (o.archived_at IS NOT NULL)=false ORDER BY n.title,n.id LIMIT 51`, nil},
		{"project_subtree", `WITH RECURSIVE subtree AS (SELECT id,parent_id FROM nodes WHERE id=$1::uuid UNION ALL SELECT n.id,n.parent_id FROM nodes n JOIN subtree s ON n.parent_id=s.id WHERE n.tenant_id=current_setting('aeon.tenant_id')::uuid AND n.deleted_at IS NULL) SELECT count(*) FROM subtree`, []any{project}},
		{"search_lexical", `SELECT node_id,score FROM aeon_search_nodes($1,NULL::halfvec(1536),NULL::text,NULL::uuid,NULL::text,50)`, []any{"Ticket"}},
	} {
		explain(t, ctx, d, tid, plan.name, plan.sql, plan.args...)
	}
}

func call(t *testing.T, mux *http.ServeMux, p tenant.Principal, path string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r = r.WithContext(tenant.WithPrincipal(r.Context(), p))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: HTTP %d: %s", path, w.Code, w.Body.String())
	}
}

func explain(t *testing.T, ctx context.Context, d *dbtest.DB, tid, name, query string, args ...any) {
	t.Helper()
	err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			fmt.Printf("PLAN %s %s\n", name, line)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
