// SPDX-License-Identifier: AGPL-3.0-only

package inbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
)

type routineRoundTrip func(*http.Request) (*http.Response, error)

func (f routineRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRoutineDispatcherUsesEncryptedTargetAndFencedCompletion(t *testing.T) {
	w, m, project, _ := messagingWorld(t)
	const senderKey = "fixture-routine-sender-key"
	const targetRef = "https://8.8.8.8/fixture-hook"
	_, err := m.storeTarget(t.Context(), w.admin, project, targetInput{Address: "grok_bot:worker", Adapter: "grok_bot_routine", Kind: "https_webhook", Ref: targetRef, Secret: senderKey, Role: "primary", MaximumLevel: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	in := compatInput("grok_bot:worker", "routine-once")
	in.Level = "steer"
	message := mustCompatSend(t, m, w.sender, project, in)
	called := 0
	dispatcher := &RoutineDispatcher{m: m, client: &http.Client{Transport: routineRoundTrip(func(req *http.Request) (*http.Response, error) {
		called++
		if req.URL.String() != targetRef || req.Header.Get("Authorization") != "Bearer "+senderKey || req.Header.Get("Idempotency-Key") == "" {
			t.Fatal("routine request lost private binding")
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var wake routineWake
		if err := json.Unmarshal(body, &wake); err != nil {
			t.Fatal(err)
		}
		if wake.Event != "agent_message.available" || wake.MessageID != message.ID || wake.EffectiveLevel != "simple" || wake.FallbackReason != "unsupported" || !strings.Contains(wake.Content, "SECURITY NOTICE") || !strings.Contains(wake.Content, in.Body) || strings.Contains(string(body), senderKey) || strings.Contains(string(body), targetRef) {
			t.Fatal("routine wake content or redaction mismatch")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}}
	worked, err := dispatcher.DispatchOne(t.Context(), w.agent.TenantID)
	if err != nil || !worked || called != 1 {
		t.Fatalf("dispatch worked=%t calls=%d err=%v", worked, called, err)
	}
	worked, err = dispatcher.DispatchOne(t.Context(), w.agent.TenantID)
	if err != nil || worked || called != 1 {
		t.Fatalf("duplicate dispatch worked=%t calls=%d err=%v", worked, called, err)
	}
	var state, effective, reason string
	err = db.InTenant(dbtest.Seed(context.Background()), w.db.App, w.agent.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT state,effective_level,fallback_reason FROM inbox_message_deliveries WHERE message_id=$1::uuid`, message.ID).Scan(&state, &effective, &reason)
	})
	if err != nil || state != "delivered" || effective != "simple" || reason != "unsupported" {
		t.Fatalf("delivery state=%s level=%s reason=%s err=%v", state, effective, reason, err)
	}
}
