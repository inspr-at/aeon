// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRemoteUsesAeonRunAndInboxContract(t *testing.T) {
	seen := map[string]bool{}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer scoped-key" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/runs/run/telemetry" && (r.Header.Get("X-Aeon-Daemon-ID") != "daemon" || r.Header.Get("X-Aeon-Daemon-Generation") != "generation") {
			t.Errorf("telemetry lacks daemon fencing headers")
		}
		mu.Lock()
		seen[r.Method+" "+r.URL.Path] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me":
			_, _ = w.Write([]byte(`{"tenant":{"id":"tenant"},"principal":{"id":"agent","tenant_id":"tenant","kind":"agent"}}`))
		case "/api/runs/queued":
			_, _ = w.Write([]byte(`[{"id":"run","agent_principal_id":"agent"}]`))
		case "/api/agent-accounts/route":
			_, _ = w.Write([]byte(`{"account_id":"account","account_key":"local","daemon_id":"daemon","reservations":[{"reservation_id":"reservation"}]}`))
		case "/api/inbox/messages":
			_, _ = w.Write([]byte(`{"items":[],"next_after":0}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	r := NewRemote(server.URL, "scoped-key")
	ctx := context.Background()
	if tenant, principal, err := r.Identity(ctx); err != nil || tenant != "tenant" || principal != "agent" {
		t.Fatalf("identity: %s %s %v", tenant, principal, err)
	}
	if runs, err := r.Queued(ctx); err != nil || len(runs) != 1 {
		t.Fatalf("queued: %#v %v", runs, err)
	}
	if _, err := r.Route(ctx, "run", map[string]int64{"requests": 1}); err != nil {
		t.Fatal(err)
	}
	if err := r.Claim(ctx, "run", "daemon", "generation", []string{"reservation"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Report(ctx, "run", Telemetry{Sequence: 1, Kind: "heartbeat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Inbox(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err := r.Ack(ctx, "message"); err != nil {
		t.Fatal(err)
	}
	if err := r.Probe(ctx, "account", "daemon", "generation", true); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEvidence(ctx, "order", "run", "answer"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	for _, path := range []string{"POST /api/runs/run/claim", "POST /api/runs/run/telemetry", "GET /api/inbox/messages",
		"POST /api/inbox/messages/message/ack", "POST /api/agent-accounts/account/probe", "POST /api/work-orders/order/evidence"} {
		if !seen[path] {
			t.Errorf("missing %s", path)
		}
	}
	mu.Unlock()
	if err := ValidateBaseURL("http://example.com"); err == nil {
		t.Fatal("remote cleartext accepted")
	}
	if err := ValidateBaseURL("https://example.com"); err != nil {
		t.Fatal(err)
	}
}
