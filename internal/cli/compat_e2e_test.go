// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/auth"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/inbox"
	"github.com/inspr-at/aeon/internal/modelregistry"
	"github.com/inspr-at/aeon/internal/nodes"
	"github.com/inspr-at/aeon/internal/search"
)

func TestUnsupportedCompatCommands(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "no-config.yaml")
	if len(unsupportedCompat) == 0 {
		t.Fatal("unsupported list is empty")
	}
	for _, tc := range unsupportedCompat {
		if tc.Reason == "" {
			t.Errorf("%s: missing reason", tc.Name)
			continue
		}
		if len(tc.Args) == 0 {
			continue
		}
		args := append([]string{"paimos", "--config", missing}, tc.Args...)
		code, out, errOut := runCLI(args, "")
		if code != 3 || !strings.Contains(errOut, tc.Reason) {
			t.Errorf("%s: code %d out %q err %q", tc.Name, code, out, errOut)
		}
		if strings.Contains(out+errOut, "pm.barta.cm") || strings.Contains(out+errOut, "barta.cm") {
			t.Errorf("%s contacted a classic host: %s", tc.Name, out+errOut)
		}
	}
}

func TestCompatEndToEnd(t *testing.T) {
	isolate(t)
	ctx := t.Context()
	opened := dbtest.Open(t)
	if err := db.EnsureTenant(ctx, opened.App, "aeon", "Aeon"); err != nil {
		t.Fatal(err)
	}
	authMod, err := auth.New(auth.Config{
		Env:                 "dev",
		SessionKey:          bytes.Repeat([]byte{9}, 32),
		PublicURL:           "http://127.0.0.1",
		BootstrapTenantSlug: "aeon",
		BootstrapAdminEmail: "admin@example.com",
	}, opened.App)
	if err != nil {
		t.Fatal(err)
	}
	api := &httpapi.Server{
		Pool: opened.App,
		Modules: []httpapi.Module{
			authMod,
			nodes.New(opened.App, nodes.SQLWriter{}),
			search.New(opened.App, nil),
			modelregistry.New(opened.App),
			inbox.New(opened.App),
		},
		Middleware: []func(http.Handler) http.Handler{authMod.Middleware},
	}
	var hits atomic.Int64
	handler := api.Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, splitErr := net.SplitHostPort(r.Host)
		if splitErr != nil || host != "127.0.0.1" {
			t.Errorf("request host %q", r.Host)
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		hits.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	parsed, err := url.Parse(srv.URL)
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("server url %s", srv.URL)
	}

	missing := filepath.Join(t.TempDir(), "no-config.yaml")
	t.Setenv("PAIMOS_URL", srv.URL)
	before := hits.Load()
	for _, tc := range unsupportedCompat {
		if len(tc.Args) == 0 {
			continue
		}
		args := append([]string{"paimos", "--config", missing}, tc.Args...)
		code, out, errOut := runCLI(args, "")
		if code != 3 || !strings.Contains(errOut, tc.Reason) {
			t.Errorf("%s: code %d out %q err %q", tc.Name, code, out, errOut)
		}
	}
	if hits.Load() != before {
		t.Fatalf("unsupported commands made %d requests", hits.Load()-before)
	}

	worker := mintAgent(t, srv.URL, "worker")
	peer := mintAgent(t, srv.URL, "peer")
	seedProject(t, srv.URL, worker.Token)
	t.Setenv("PAIMOS_API_KEY", worker.Token)

	code, out, errOut := runCLI([]string{"paimos", "--config", missing, "whoami"}, "")
	if code != 0 || !strings.Contains(out, "principal: worker (agent)") || !strings.Contains(out, "tenant: Aeon (aeon)") {
		t.Fatalf("whoami code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "issue", "create", "--project", "AEON", "--title", "Calendar compat", "--description", "ship the compat test", "--priority", "high", "--type", "ticket"}, "")
	if code != 0 || !strings.Contains(out, "✓ created AEON-") || !strings.Contains(out, "Calendar compat") {
		t.Fatalf("create code %d out %q err %q", code, out, errOut)
	}
	key := issueKey(out)
	if key == "" {
		t.Fatalf("create out %q", out)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "issue", "get", key}, "")
	if code != 0 || !strings.Contains(out, key+"  Calendar compat") || !strings.Contains(out, "type:     ticket") || !strings.Contains(out, "status:   open") || !strings.Contains(out, "priority: high") {
		t.Fatalf("get code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "--json", "issue", "list", "-p", "AEON", "--priority", "high"}, "")
	if code != 0 {
		t.Fatalf("list code %d err %s", code, errOut)
	}
	var listed struct {
		Issues []issueView `json:"issues"`
		Total  int         `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Total < 1 || listed.Issues[0].IssueKey != key || listed.Issues[0].Status != "open" || listed.Issues[0].Priority != "high" {
		t.Fatalf("list %+v", listed)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "issue", "update", key, "--status", "in-progress"}, "")
	if code != 0 || !strings.Contains(out, "✓ "+key+": open → in-progress") {
		t.Fatalf("update code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "issue", "comment", key, "--body", "noted"}, "")
	if code != 0 || !strings.Contains(out, "✓ commented on "+key) {
		t.Fatalf("comment code %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "--json", "issue", "get", key}, "")
	if code != 0 {
		t.Fatalf("json get code %d err %s", code, errOut)
	}
	var got issueView
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.IssueKey != key || got.Status != "in-progress" || len(got.Comments) != 1 || got.Comments[0] != "noted" {
		t.Fatalf("json get %+v", got)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "issue", "search", "Calendar", "-p", "AEON"}, "")
	if code != 0 || !strings.Contains(out, "KEY           TYPE     STATUS         TITLE") || !strings.Contains(out, key) || !strings.Contains(out, "ticket") {
		t.Fatalf("search code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "knowledge", "create", "--type", "memory", "--slug", "note", "--project", "AEON", "--title", "Note", "--body", "remember this"}, "")
	if code != 0 || !strings.Contains(out, "✓ created memory/note (") {
		t.Fatalf("knowledge create code %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "knowledge", "list", "--project", "AEON", "--type", "memory"}, "")
	if code != 0 || !strings.Contains(out, "TYPE             SLUG") || !strings.Contains(out, "memory") || !strings.Contains(out, "note") {
		t.Fatalf("knowledge list code %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "knowledge", "get", "memory", "note", "--project", "AEON"}, "")
	if code != 0 || !strings.Contains(out, "memory/note (") || !strings.Contains(out, "title:  Note") || !strings.Contains(out, "status: open") {
		t.Fatalf("knowledge get code %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "knowledge", "update", "memory", "note", "--project", "AEON", "--title", "Noted"}, "")
	if code != 0 || !strings.Contains(out, "✓ updated memory/note (") {
		t.Fatalf("knowledge update code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "model", "resolve", "build"}, "")
	if code != 0 || !strings.Contains(out, " — ") || !strings.Contains(out, "'{prompt}'") || !strings.Contains(out, "selected") {
		t.Fatalf("resolve code %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "model", "resolve", "review-gate", "--author-family", "xai"}, "")
	line, _, _ := strings.Cut(out, "\n")
	if code != 0 || !strings.Contains(out, "'{prompt}'") || strings.HasSuffix(strings.TrimSpace(line), "xai") {
		t.Fatalf("review-gate code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "session", "start", "--project", "AEON", "--agent", "worker"}, "")
	if code != 0 || !strings.Contains(out, "export PAIMOS_AGENT_NAME=worker\n") || !regexp.MustCompile(`export PAIMOS_SESSION_ID=[0-9a-f-]{36}\n`).MatchString(out) {
		t.Fatalf("session code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "onboard", "--project", "AEON", "--agent", "worker"}, "")
	if code != 0 || !strings.Contains(out, "# Welcome to AEON") || !strings.Contains(out, "paimos session start --project AEON --agent worker") || !strings.Contains(out, key) {
		t.Fatalf("onboard code %d out %q err %q", code, out, errOut)
	}

	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "tell", peer.PrincipalID, "--project", "AEON", "--level", "simple", "-m", "hello from compat"}, "")
	if code != 0 || !strings.Contains(out, "✓ worker → "+peer.PrincipalID+" (sent)") || !strings.Contains(out, "message: ") {
		t.Fatalf("tell code %d out %q err %q", code, out, errOut)
	}

	t.Setenv("PAIMOS_API_KEY", peer.Token)
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "listen", "--as", "peer", "--project", "AEON", "--ack"}, "")
	if code != 0 || !strings.Contains(out, "cursor=") || !strings.Contains(out, "hello from compat") || !strings.Contains(out, peer.PrincipalID) {
		t.Fatalf("listen code %d out %q err %q", code, out, errOut)
	}

	t.Setenv("PAIMOS_API_KEY", worker.Token)
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "message", "target", "set", "--principal", worker.PrincipalID, "--kind", "pull"}, "")
	if code != 0 || !strings.Contains(out, "✓ enabled target ") || !strings.Contains(out, "(pull)") {
		t.Fatalf("target set code %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = runCLI([]string{"paimos", "--config", missing, "message", "target", "list"}, "")
	if code != 0 || !strings.Contains(out, "pull") || !strings.Contains(out, "enabled") || strings.Contains(out, "http") {
		t.Fatalf("target list code %d out %q err %q", code, out, errOut)
	}

	assertEvent(t, opened, worker.TenantID, "node.created")
	assertEvent(t, opened, worker.TenantID, "node.updated")
	assertEvent(t, opened, worker.TenantID, "inbox.sent")
	assertEvent(t, opened, worker.TenantID, "inbox.acked")
	assertEvent(t, opened, worker.TenantID, "inbox.target_created")
	assertEvent(t, opened, worker.TenantID, "model.registry_seeded")
}

func TestMessagingCompatEndToEnd(t *testing.T) {
	isolate(t)
	opened := dbtest.Open(t)
	if err := db.EnsureTenant(t.Context(), opened.App, "aeon", "Aeon"); err != nil {
		t.Fatal(err)
	}
	authMod, err := auth.New(auth.Config{Env: "dev", SessionKey: bytes.Repeat([]byte{8}, 32), PublicURL: "http://127.0.0.1", BootstrapTenantSlug: "aeon", BootstrapAdminEmail: "admin@example.com"}, opened.App)
	if err != nil {
		t.Fatal(err)
	}
	messaging, err := inbox.NewMessaging(opened.App, bytes.Repeat([]byte{6}, 32))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer((&httpapi.Server{Pool: opened.App, Modules: []httpapi.Module{authMod, nodes.New(opened.App, nodes.SQLWriter{}), inbox.New(opened.App), messaging}, Middleware: []func(http.Handler) http.Handler{authMod.Middleware}}).Handler())
	defer srv.Close()
	sender := mintAgent(t, srv.URL, "sender")
	receiver := mintAgent(t, srv.URL, "receiver")
	seedProject(t, srv.URL, sender.Token)
	t.Setenv("PAIMOS_URL", srv.URL)
	t.Setenv("PAIMOS_API_KEY", sender.Token)
	missing := filepath.Join(t.TempDir(), "missing")
	call := func(args []string) string {
		t.Helper()
		code, out, stderr := runMessagingCLI(append([]string{"--config", missing, "--json"}, args...), "")
		if code != 0 {
			t.Fatalf("messaging command %s: code %d, %s", args[0], code, stderr)
		}
		return out
	}
	first := call([]string{"tell", "codex:receiver", "--project", "AEON", "--expects-reply", "--idempotency-key", "retry-key", "-m", "reply with validation"})
	var sent inbox.CompatMessage
	if err := json.Unmarshal([]byte(first), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.ReplyObligation != "open" {
		t.Fatal("no reply obligation")
	}
	call([]string{"tell", "codex:receiver", "--project", "AEON", "--action-request", "-m", "held action fixture"})
	t.Setenv("PAIMOS_API_KEY", receiver.Token)
	page := call([]string{"listen", "--as", "codex:receiver", "--project", "AEON", "--ack"})
	if !strings.Contains(page, "reply with validation") || strings.Contains(page, "held action fixture") {
		t.Fatal("listen exposed held content or lost accepted content")
	}
	page = call([]string{"listen", "--as", "codex:receiver", "--project", "AEON"})
	if strings.Contains(page, "reply with validation") {
		t.Fatal("JSON --ack did not acknowledge")
	}
	call([]string{"tell", "paimos:sender", "--project", "AEON", "--reply-to", sent.ID, "-m", "validation complete"})
	t.Setenv("PAIMOS_API_KEY", sender.Token)
	replay := call([]string{"tell", "codex:receiver", "--project", "AEON", "--expects-reply", "--idempotency-key", "retry-key", "-m", "reply with validation"})
	var replayed inbox.CompatMessage
	if err := json.Unmarshal([]byte(replay), &replayed); err != nil {
		t.Fatal(err)
	}
	if replayed.ReplyObligation != "closed" {
		t.Fatal("reply did not close obligation")
	}
	assertEvent(t, opened, sender.TenantID, "inbox.reply_obligation_opened")
	assertEvent(t, opened, sender.TenantID, "inbox.reply_obligation_closed")
	assertEvent(t, opened, sender.TenantID, "inbox.delivery_queued")
}

type mintedKey struct {
	Token       string
	PrincipalID string
	TenantID    string
}

func mintAgent(t *testing.T, base, name string) mintedKey {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	hc := &http.Client{Jar: jar}
	status, raw := doJSON(t, hc, http.MethodPost, base+"/api/auth/dev-login", `{"email":"admin@example.com","tenant":"aeon"}`, nil)
	if status != http.StatusOK {
		t.Fatalf("dev login %d %s", status, raw)
	}
	var me struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		t.Fatal(err)
	}
	status, raw = doJSON(t, hc, http.MethodPost, base+"/api/agent-keys", `{"name":"`+name+`","scopes":["inbox.send"]}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("agent key %s %d %s", name, status, raw)
	}
	var created struct {
		Token       string `json:"token"`
		PrincipalID string `json:"principal_id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.PrincipalID == "" || me.Tenant.ID == "" {
		t.Fatalf("mint %+v tenant %s", created, me.Tenant.ID)
	}
	return mintedKey{Token: created.Token, PrincipalID: created.PrincipalID, TenantID: me.Tenant.ID}
}

func seedProject(t *testing.T, base, token string) {
	t.Helper()
	hc := &http.Client{}
	authz := http.Header{"Authorization": {"Bearer " + token}}
	status, raw := doJSON(t, hc, http.MethodGet, base+"/api/kinds", "", authz)
	if status != http.StatusOK {
		t.Fatalf("kinds %d %s", status, raw)
	}
	var page struct {
		Items []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	var kindID string
	for _, item := range page.Items {
		if item.Slug == "project" {
			kindID = item.ID
		}
	}
	if kindID == "" {
		t.Fatal("project kind missing")
	}
	body := `{"kind_id":"` + kindID + `","title":"AEON","body":"Compat project","fields":{"project_key":"AEON"}}`
	status, raw = doJSON(t, hc, http.MethodPost, base+"/api/nodes", body, authz)
	if status != http.StatusCreated {
		t.Fatalf("project %d %s", status, raw)
	}
}

func doJSON(t *testing.T, hc *http.Client, method, url, body string, header http.Header) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	res, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, raw
}

func issueKey(out string) string {
	const mark = "✓ created "
	i := strings.Index(out, mark)
	if i < 0 {
		return ""
	}
	rest := out[i+len(mark):]
	key, _, _ := strings.Cut(rest, " ")
	return key
}

func assertEvent(t *testing.T, opened *dbtest.DB, tenantID, eventType string) {
	t.Helper()
	var n int
	err := db.InTenant(context.Background(), opened.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE type = $1`, eventType).Scan(&n)
	})
	if err != nil {
		t.Fatalf("count %s: %v", eventType, err)
	}
	if n < 1 {
		t.Fatalf("no %s event", eventType)
	}
}
