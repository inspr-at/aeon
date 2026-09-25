// SPDX-License-Identifier: AGPL-3.0-only

package inbox

import (
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/jackc/pgx/v5"
)

func TestReceiverDeliveryClaimCompleteAndCursor(t *testing.T) {
	w, m, project, srv := messagingWorld(t)
	target := targetInput{Address: "codex:worker", Adapter: "codex", Kind: "codex_thread", Ref: "private-thread-fixture", Role: "primary", MaximumLevel: "steer"}
	created, err := m.storeTarget(t.Context(), w.admin, project, target)
	if err != nil {
		t.Fatal(err)
	}
	in := compatInput("codex:worker", "delivery-once")
	in.Level = "steer"
	msg := mustCompatSend(t, m, w.sender, project, in)
	if msg.DeliveryTarget == nil || msg.DeliveryTarget.Primary == nil || msg.DeliveryTarget.Primary.BindingID != created.ID || msg.DeliveryTarget.Primary.Kind != "codex_thread" || msg.DeliveryTarget.SimpleFallback != nil {
		t.Fatal("message lost its redacted target snapshot")
	}
	base := "/api/projects/" + project
	status, _ := compatPost(t, srv, w.sender, base+"/messages/delivery-claim", claimInput{To: in.To, Adapter: "codex"})
	if status != 403 {
		t.Fatalf("sender claimed receiver delivery: %d", status)
	}
	status, data := compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: in.To, Adapter: "claude_resume"})
	if status != 200 || mustJSON[DeliveryWork](t, data).State != "foreign_worker" {
		t.Fatalf("foreign worker %d %s", status, data)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: in.To, Adapter: "codex"})
	if status != 200 {
		t.Fatalf("claim %d %s", status, data)
	}
	work := mustJSON[DeliveryWork](t, data)
	if work.Message == nil || work.Message.ID != msg.ID || work.TargetRef != target.Ref || work.LeaseToken == "" || work.Cursor != msg.SentEventID {
		t.Fatal("claim lost receiver work")
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: in.To, Adapter: "codex"})
	if status != 200 || mustJSON[DeliveryWork](t, data).State != "leased" {
		t.Fatalf("duplicate claim %d %s", status, data)
	}
	completed := completeInput{ID: work.ID, LeaseToken: work.LeaseToken, EffectiveLevel: "steer"}
	status, _ = compatPost(t, srv, w.agent, base+"/messages/delivery-complete", completeInput{ID: work.ID, LeaseToken: work.LeaseToken, EffectiveLevel: "simple"})
	if status != 409 {
		t.Fatalf("steer downgrade without reason: %d", status)
	}
	status, _ = compatPost(t, srv, w.sender, base+"/messages/delivery-complete", completed)
	if status != 404 {
		t.Fatalf("sender completed recipient delivery: %d", status)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-complete", completed)
	if status != 200 || mustJSON[MessageDelivery](t, data).State != "delivered" {
		t.Fatalf("complete %d %s", status, data)
	}
	status, _ = compatPost(t, srv, w.agent, base+"/messages/delivery-complete", completed)
	if status != 200 {
		t.Fatalf("exact replay %d", status)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: in.To, Adapter: "codex"})
	if status != 200 || strings.TrimSpace(string(data)) != "null" {
		t.Fatalf("completed work replayed %d %s", status, data)
	}
	status, data = do(t, srv, w.admin.ID, "GET", base+"/message-deliveries", "", nil)
	if status != 200 || strings.Contains(string(data), target.Ref) || strings.Contains(string(data), in.Body) || strings.Contains(string(data), work.LeaseToken) {
		t.Fatal("redacted ledger leaked private delivery fields")
	}
	var cursor int64
	err = db.InTenant(t.Context(), w.db.App, w.agent.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT last_event_id FROM inbox_message_cursors WHERE project_id=$1::uuid AND principal_id=$2::uuid AND address=$3 AND adapter='codex'`, project, w.agent.ID, in.To).Scan(&cursor)
	})
	if err != nil || cursor != msg.SentEventID {
		t.Fatalf("durable cursor %d: %v", cursor, err)
	}
}

func TestCompatAckResolvesInboxIDAndProject(t *testing.T) {
	w, m, project, srv := messagingWorld(t)
	in := compatInput("codex:worker", "manual-read")
	msg := mustCompatSend(t, m, w.sender, project, in)
	path := "/api/projects/" + project + "/messages/" + msg.ID + "/ack"
	status, data := do(t, srv, w.sender.ID, "POST", path, "", nil)
	if status != 404 && status != 403 {
		t.Fatalf("sender ack %d %s", status, data)
	}
	for range 2 {
		status, data = do(t, srv, w.agent.ID, "POST", path, "", nil)
		if status != 200 || !strings.Contains(string(data), `"acked":true`) {
			t.Fatalf("recipient ack %d %s", status, data)
		}
	}
	var ackCount int
	err := db.InTenant(t.Context(), w.db.App, w.agent.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type='inbox.acked' AND after->>'id'=(SELECT inbox_message_id::text FROM inbox_compat_messages WHERE id=$1::uuid)`, msg.ID).Scan(&ackCount)
	})
	if err != nil || ackCount != 1 {
		t.Fatalf("ack event count %d: %v", ackCount, err)
	}
}

func TestManagedDeliveryReroutesToFrozenSimpleFallback(t *testing.T) {
	w, m, project, srv := messagingWorld(t)
	address := "codex:worker"
	fallback, err := m.storeTarget(t.Context(), w.admin, project, targetInput{Address: address, Adapter: "codex", Kind: "codex_thread", Ref: "fallback-thread", Role: "simple_fallback", MaximumLevel: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	primary, err := m.storeTarget(t.Context(), w.admin, project, targetInput{Address: address, Adapter: "agentd_codex", Kind: "agentd_session", Ref: `{"socket":"/private/tmp/fixture.sock","session_id":"00000000-0000-4000-8000-000000000010"}`, Role: "primary", MaximumLevel: "steer"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + project
	simple := compatInput(address, "managed-simple")
	simple.Level = "simple"
	mustCompatSend(t, m, w.sender, project, simple)
	status, data := compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: address, Adapter: "codex"})
	work := mustJSON[DeliveryWork](t, data)
	if status != 200 || work.TargetRef != "fallback-thread" || work.FallbackReason != "not_steerable" {
		t.Fatalf("simple reroute %d %s", status, data)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-complete", completeInput{ID: work.ID, LeaseToken: work.LeaseToken, EffectiveLevel: "simple", FallbackReason: work.FallbackReason})
	if status != 200 || mustJSON[MessageDelivery](t, data).TargetID == nil || *mustJSON[MessageDelivery](t, data).TargetID != primary.ID {
		t.Fatalf("simple complete %d %s", status, data)
	}
	steer := compatInput(address, "managed-steer")
	steer.Level = "steer"
	mustCompatSend(t, m, w.sender, project, steer)
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: address, Adapter: "agentd_codex"})
	work = mustJSON[DeliveryWork](t, data)
	if status != 200 || work.Adapter != "agentd_codex" || work.LeaseToken == "" {
		t.Fatalf("managed claim %d %s", status, data)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-unavailable", unavailableInput{ID: work.ID, LeaseToken: work.LeaseToken, FallbackReason: "idle"})
	if status != 200 {
		t.Fatalf("managed unavailable %d %s", status, data)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-claim", claimInput{To: address, Adapter: "codex"})
	work = mustJSON[DeliveryWork](t, data)
	if status != 200 || work.TargetRef != "fallback-thread" || work.FallbackReason != "idle" {
		t.Fatalf("fallback claim %d %s", status, data)
	}
	status, data = compatPost(t, srv, w.agent, base+"/messages/delivery-complete", completeInput{ID: work.ID, LeaseToken: work.LeaseToken, EffectiveLevel: "simple", FallbackReason: "idle"})
	delivery := mustJSON[MessageDelivery](t, data)
	if status != 200 || delivery.FallbackTargetID == nil || *delivery.FallbackTargetID != fallback.ID || delivery.TargetID == nil || *delivery.TargetID != primary.ID || delivery.FallbackReason != "idle" {
		t.Fatalf("fallback complete %d %s", status, data)
	}
}
