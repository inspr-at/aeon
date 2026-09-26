// SPDX-License-Identifier: AGPL-3.0-only

package inbox

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestInboxReceipt(t *testing.T) {
	t.Parallel()
	perm, ok := authz.Lookup("inbox.receipt")
	if !ok || !perm.AgentGrantable {
		t.Fatal("inbox.receipt must be an agent-grantable permission")
	}
	w, m, project, srv := messagingWorld(t)
	const secret = "RECEIPT-BODY-SECRET-do-not-leak"
	const key = "receipt-idem"

	status, body := do(t, srv, w.sender.ID, http.MethodPost, "/api/inbox/messages", sendJSON(w.recipient.ID, secret, key, nil, nil), nil)
	if status != 201 {
		t.Fatalf("send %d %s", status, body)
	}
	first := mustJSON[Message](t, body)
	status, body = do(t, srv, w.sender.ID, http.MethodPost, "/api/inbox/messages", sendJSON(w.recipient.ID, secret, key, nil, nil), nil)
	replay := mustJSON[Message](t, body)
	if status != 201 || replay.ID != first.ID {
		t.Fatalf("idempotent replay %d %s", status, body)
	}
	if countSQL(t, w.db.App, w.sender.TenantID, `SELECT count(*) FROM inbox_receipts WHERE message_id=$1::uuid`, first.ID) != 1 {
		t.Fatal("replay wrote another receipt")
	}
	if countSQL(t, w.db.App, w.sender.TenantID, `SELECT count(*) FROM events WHERE type='inbox.receipt_queued' AND after->>'message_id'=$1`, first.ID) != 1 {
		t.Fatal("replay wrote another receipt event")
	}

	rec := mustReceipt(t, srv, w.sender.ID, first.ID)
	if rec.MessageID != first.ID || rec.IdempotencyKey != key || rec.Tenant != w.db.Name+"-a" ||
		rec.SenderPrincipalID != w.sender.ID || rec.RecipientPrincipalID != w.recipient.ID ||
		rec.State != "queued" || rec.HandedOffAt != nil || rec.FailureReason != "" ||
		rec.TargetID != nil || rec.TargetVersion != nil || rec.Adapter != "" || rec.Address != "" || rec.EffectiveLevel != "" {
		t.Fatalf("accepted receipt %+v", rec)
	}
	if strings.Contains(string(mustReceiptRaw(t, srv, w.sender.ID, first.ID)), secret) {
		t.Fatal("receipt leaked the body")
	}
	_, hidden := do(t, srv, w.sender.ID, http.MethodGet, "/api/inbox/messages/00000000-0000-4000-8000-000000000000/receipt", "", nil)
	for _, principal := range []tenant.Principal{w.recipient, w.admin, w.agent, w.outsider} {
		status, body = do(t, srv, principal.ID, http.MethodGet, "/api/inbox/messages/"+first.ID+"/receipt", "", nil)
		if status != 404 || string(body) != string(hidden) || strings.Contains(string(body), first.ID) || strings.Contains(string(body), secret) {
			t.Fatalf("non-sender %s %d %s", principal.Name, status, body)
		}
	}
	status, _ = do(t, srv, w.sender.ID, http.MethodGet, "/api/inbox/messages/not-a-uuid/receipt", "", nil)
	if status != 404 {
		t.Fatalf("bad id %d", status)
	}

	const senderKey = "fixture-routine-sender-key"
	const targetRef = "https://8.8.8.8/fixture-hook"
	target, err := m.storeTarget(t.Context(), w.admin, project, targetInput{
		Address: "grok_bot:worker", Adapter: "grok_bot_routine", Kind: "https_webhook",
		Ref: targetRef, Secret: senderKey, Role: "primary", MaximumLevel: "simple",
	})
	if err != nil {
		t.Fatal(err)
	}
	respond := func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}
	dispatcher := &RoutineDispatcher{m: m, client: &http.Client{Transport: routineRoundTrip(func(req *http.Request) (*http.Response, error) {
		return respond(req)
	})}}

	handed := sendCompat(t, m, w.sender, project, "grok_bot:worker", "hand-off", secret)
	queued := mustReceipt(t, srv, w.sender.ID, handed.ID)
	if queued.State != "queued" || queued.HandedOffAt != nil || queued.EffectiveLevel != "" ||
		queued.TargetID == nil || *queued.TargetID != target.ID || queued.TargetVersion == nil || *queued.TargetVersion != target.Version ||
		queued.Adapter != "grok_bot_routine" || queued.Address != "grok_bot:worker" ||
		queued.IdempotencyKey != "compat/"+handed.ID || queued.SenderPrincipalID != w.sender.ID {
		t.Fatalf("queued before confirmation %+v", queued)
	}
	if strings.Contains(string(mustReceiptRaw(t, srv, w.sender.ID, handed.ID)), secret) ||
		strings.Contains(string(mustReceiptRaw(t, srv, w.sender.ID, handed.ID)), senderKey) ||
		strings.Contains(string(mustReceiptRaw(t, srv, w.sender.ID, handed.ID)), targetRef) {
		t.Fatal("queued receipt leaked private material")
	}

	worked, err := dispatcher.DispatchOne(t.Context(), w.sender.TenantID)
	if err != nil || !worked {
		t.Fatalf("confirm worked=%t err=%v", worked, err)
	}
	confirmed := mustReceipt(t, srv, w.sender.ID, handed.ID)
	if confirmed.State != "handed_off" || confirmed.HandedOffAt == nil || confirmed.EffectiveLevel != "simple" ||
		confirmed.FailureReason != "" || confirmed.TargetID == nil || *confirmed.TargetID != target.ID {
		t.Fatalf("handed off %+v", confirmed)
	}
	if _, parseErr := time.Parse(time.RFC3339, *confirmed.HandedOffAt); parseErr != nil {
		t.Fatal(parseErr)
	}
	again := mustReceipt(t, srv, w.sender.ID, handed.ID)
	if again.State != "handed_off" || *again.HandedOffAt != *confirmed.HandedOffAt {
		t.Fatalf("handed_off_at moved %+v", again)
	}
	if err := db.InTenant(dbtest.Seed(t.Context()), w.db.App, w.sender.TenantID, func(tx pgx.Tx) error {
		return advanceReceipt(t.Context(), tx, w.agent, handed.ID, "failed", "", "late", receiptTarget{})
	}); err != nil {
		t.Fatal(err)
	}
	if stayed := mustReceipt(t, srv, w.sender.ID, handed.ID); stayed.State != "handed_off" || *stayed.HandedOffAt != *confirmed.HandedOffAt {
		t.Fatalf("handed_off moved backward %+v", stayed)
	}
	err = db.InTenant(dbtest.Seed(t.Context()), w.db.App, w.sender.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE inbox_receipts SET state='failed', failure_reason='rewrite', handed_off_at=NULL WHERE message_id=$1::uuid`, handed.ID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "monotonic") {
		t.Fatalf("trigger allowed rewrite: %v", err)
	}
	if countSQL(t, w.db.App, w.sender.TenantID, `SELECT count(*) FROM events WHERE type='inbox.receipt_handed_off' AND after->>'message_id'=$1`, handed.ID) != 1 {
		t.Fatal("hand-off event was not written once")
	}

	retryMsg := sendCompat(t, m, w.sender, project, "grok_bot:worker", "retry-off", secret)
	respond = func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`later`))}, nil
	}
	worked, err = dispatcher.DispatchOne(t.Context(), w.sender.TenantID)
	if err == nil || !worked {
		t.Fatalf("retryable failure worked=%t err=%v", worked, err)
	}
	if got := mustReceipt(t, srv, w.sender.ID, retryMsg.ID); got.State != "queued" || got.HandedOffAt != nil || got.FailureReason != "" {
		t.Fatalf("retryable failure became terminal %+v", got)
	}
	if err := db.InTenant(dbtest.Seed(t.Context()), w.db.App, w.sender.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE inbox_message_deliveries SET lease_until=NULL WHERE message_id=$1::uuid`, retryMsg.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	respond = func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}
	worked, err = dispatcher.DispatchOne(t.Context(), w.sender.TenantID)
	if err != nil || !worked {
		t.Fatalf("confirm after retry worked=%t err=%v", worked, err)
	}
	if got := mustReceipt(t, srv, w.sender.ID, retryMsg.ID); got.State != "handed_off" || got.HandedOffAt == nil || got.EffectiveLevel != "simple" {
		t.Fatalf("confirmed after retry %+v", got)
	}

	failedMsg := sendCompat(t, m, w.sender, project, "grok_bot:worker", "fail-off", secret)
	if got := mustReceipt(t, srv, w.sender.ID, failedMsg.ID); got.State != "queued" {
		t.Fatalf("failure candidate %+v", got)
	}
	respond = func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`no`))}, nil
	}
	worked, err = dispatcher.DispatchOne(t.Context(), w.sender.TenantID)
	if err == nil || !worked {
		t.Fatalf("terminal failure worked=%t err=%v", worked, err)
	}
	failed := mustReceipt(t, srv, w.sender.ID, failedMsg.ID)
	if failed.State != "failed" || failed.FailureReason != "http_error" || failed.HandedOffAt != nil || failed.Adapter != "grok_bot_routine" {
		t.Fatalf("failed receipt %+v", failed)
	}
	if strings.Contains(string(mustReceiptRaw(t, srv, w.sender.ID, failedMsg.ID)), secret) {
		t.Fatal("failed receipt leaked the body")
	}
	if err := db.InTenant(dbtest.Seed(t.Context()), w.db.App, w.sender.TenantID, func(tx pgx.Tx) error {
		return advanceReceipt(t.Context(), tx, w.agent, failedMsg.ID, "handed_off", "simple", "", receiptTarget{ID: &target.ID, Version: &target.Version, Adapter: target.Adapter, Address: target.Address})
	}); err != nil {
		t.Fatal(err)
	}
	if stayed := mustReceipt(t, srv, w.sender.ID, failedMsg.ID); stayed.State != "failed" || stayed.FailureReason != "http_error" || stayed.HandedOffAt != nil {
		t.Fatalf("failed moved backward %+v", stayed)
	}

	leaked := eventText(t, w.db.App, w.sender.TenantID, "inbox.receipt_queued") +
		eventText(t, w.db.App, w.sender.TenantID, "inbox.receipt_handed_off") +
		eventText(t, w.db.App, w.sender.TenantID, "inbox.receipt_failed")
	if strings.Contains(leaked, secret) || strings.Contains(leaked, senderKey) || strings.Contains(leaked, targetRef) {
		t.Fatal("receipt event leaked private material")
	}
}

func sendCompat(t *testing.T, m *messaging, p tenant.Principal, project, to, key, body string) CompatMessage {
	t.Helper()
	in := compatInput(to, key)
	in.Body = body
	return mustCompatSend(t, m, p, project, in)
}

func mustReceipt(t *testing.T, srv *httptest.Server, principal, id string) Receipt {
	t.Helper()
	return mustJSON[Receipt](t, mustReceiptRaw(t, srv, principal, id))
}

func mustReceiptRaw(t *testing.T, srv *httptest.Server, principal, id string) []byte {
	t.Helper()
	status, body := do(t, srv, principal, http.MethodGet, "/api/inbox/messages/"+id+"/receipt", "", nil)
	if status != 200 {
		t.Fatalf("receipt %s %d %s", id, status, body)
	}
	return body
}
