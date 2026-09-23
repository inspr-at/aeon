// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFakeVendorProcess is invoked only through a private test wrapper. It
// models documented protocol acknowledgements without invoking vendor CLIs.
func TestFakeVendorProcess(t *testing.T) {
	vendor := os.Getenv("AEON_FAKE_VENDOR")
	if vendor == "" {
		return
	}
	read := bufio.NewScanner(os.Stdin)
	write := json.NewEncoder(os.Stdout)
	for read.Scan() {
		var frame struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Type   string          `json:"type"`
		}
		if json.Unmarshal(read.Bytes(), &frame) != nil {
			os.Exit(2)
		}
		id := strings.Trim(string(frame.ID), "\"")
		if vendor == "claude" {
			var command struct {
				Op            string `json:"op"`
				CorrelationID string `json:"correlation_id"`
			}
			if json.Unmarshal(read.Bytes(), &command) != nil {
				os.Exit(2)
			}
			if command.Op == "start" {
				_ = write.Encode(map[string]string{"kind": "session_started", "harness_session_id": "claude-session", "effective_model": "test-model", "model_evidence_status": "vendor_reported"})
			} else {
				_ = write.Encode(map[string]string{"kind": "control_applied", "correlation_id": command.CorrelationID})
			}
			continue
		}
		if vendor == "pi" {
			data := any(map[string]any{})
			if frame.Type == "get_state" {
				data = map[string]any{"model": map[string]string{"provider": "anthropic", "id": "test-model"}, "thinkingLevel": "high"}
			}
			if frame.Type == "clear_queue" {
				data = map[string]any{"steering": []string{"held steer"}, "followUp": []string{"held follow"}}
			}
			_ = write.Encode(map[string]any{"id": id, "type": "response", "command": frame.Type, "success": true, "data": data})
			continue
		}
		if id == "" {
			continue
		}
		result := any(map[string]any{})
		switch frame.Method {
		case "account/read":
			result = map[string]any{"account": map[string]string{"type": "chatgpt", "email": "agent@example.test"}}
		case "thread/start":
			result = map[string]any{"thread": map[string]string{"id": "thread-1"}}
		case "turn/start":
			result = map[string]any{"turn": map[string]string{"id": "turn-1", "status": "inProgress"}}
		case "turn/steer":
			result = map[string]string{"turnId": "turn-1"}
		case "session/new":
			result = map[string]any{"sessionId": "session-1", "modes": map[string]string{"currentModeId": "default"}, "models": map[string]string{"currentModelId": "test-model"}}
		case "session/prompt":
			time.Sleep(500 * time.Millisecond)
			result = map[string]string{"stopReason": "end_turn"}
		case "initialize":
			if vendor == "cursor" {
				result = map[string]int{"protocolVersion": 1}
			}
		}
		_ = write.Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": result})
	}
}

func fakeVendorPath(t *testing.T, vendor string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "fake-vendor")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = status ]; then printf '%%s\\n' '{\"status\":\"authenticated\",\"isAuthenticated\":true,\"userInfo\":{\"userId\":\"42\"}}'; exit 0; fi\nif [ \"$1\" = login ]; then printf '%%s\\n' 'Logged in using ChatGPT'; exit 0; fi\nif [ \"$1\" = auth ]; then printf '%%s\\n' '{\"loggedIn\":true}'; exit 0; fi\nAEON_FAKE_VENDOR=%s exec %q -test.run=TestFakeVendorProcess\n", vendor, exe)
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func adapterRequest(t *testing.T) StartRequest {
	t.Helper()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	return StartRequest{TenantID: "tenant", PrincipalID: "agent", Run: Run{ID: "run", WorkOrderID: "order", AgentPrincipalID: "agent"},
		Profile: Profile{ID: "profile", Harness: Codex, Model: "test-model", Effort: "high"}, AccountKey: "account", Workspace: workspace, StateRoot: state, Prompt: "hello", Generation: "generation"}
}

func TestCodexAppServerProtocolAndSteer(t *testing.T) {
	r := adapterRequest(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	a := NewCodexAdapter(fakeVendorPath(t, "codex"), map[string]string{"account": home})
	a.SetExpectedEmails(map[string]string{"account": "agent@example.test"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := a.Start(ctx, r, func(AdapterEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "steer", "follow up"); err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "interrupt", ""); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPiRPCStateAndSteer(t *testing.T) {
	r := adapterRequest(t)
	r.Profile.Harness = Pi
	r.Profile.Model = "anthropic/test-model"
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	a := NewPiAdapter(fakeVendorPath(t, "pi"), map[string]string{"account": home})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := a.Start(ctx, r, func(AdapterEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "steer", "follow up"); err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "interrupt", ""); err != nil {
		t.Fatal(err)
	}
	pi := p.(*piProcess)
	if got := pi.queue.Snapshot(); len(got) != 1 || len(got[0].FollowUp) != 1 {
		t.Fatalf("held queue: %#v", got)
	}
	if err := p.Control(ctx, "steer", "must wait"); err == nil {
		t.Fatal("steer bypassed held queue")
	}
	if err := p.Control(ctx, "resume", ""); err != nil {
		t.Fatal(err)
	}
	if got := pi.queue.Snapshot(); len(got) != 0 {
		t.Fatalf("held queue not settled: %#v", got)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCursorACPAndAccountBinding(t *testing.T) {
	r := adapterRequest(t)
	r.Profile.Harness = Cursor
	a := NewCursorAdapter(fakeVendorPath(t, "cursor"), map[string]string{"account": "42"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := a.Start(ctx, r, func(AdapterEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "steer", "unsafe"); err != ErrUnsupported {
		t.Fatalf("Cursor steer should be closed: %v", err)
	}
	if err := p.Control(ctx, "interrupt", ""); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeAndGrokFailClosedWithoutBindings(t *testing.T) {
	r := adapterRequest(t)
	if _, err := NewClaudeAdapter("/missing", "/missing", "/missing", nil).Start(context.Background(), r, func(AdapterEvent) {}); err == nil {
		t.Fatal("Claude accepted unenrolled account")
	}
	if _, err := NewGrokAdapter().Start(context.Background(), r, func(AdapterEvent) {}); err == nil {
		t.Fatal("Grok workspace run unexpectedly accepted")
	}
}

func TestPiHeldQueueRefusesOtherGeneration(t *testing.T) {
	r := adapterRequest(t)
	r.Profile.Harness = Pi
	queue, err := openPiQueue(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Put(piHeldQueue{TenantID: r.TenantID, PrincipalID: r.PrincipalID, RunID: r.Run.ID, Generation: r.Generation, Steering: []string{"held"}}); err != nil {
		t.Fatal(err)
	}
	r.Generation = "new-generation"
	if _, err := openPiQueue(r); err == nil {
		t.Fatal("old queue silently resumed")
	}
}

func TestClaudeBridgeControlProtocol(t *testing.T) {
	r := adapterRequest(t)
	r.Profile.Harness = Claude
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	path := fakeVendorPath(t, "claude")
	sdk := filepath.Join(filepath.Dir(path), "sdk.mjs")
	if err := os.WriteFile(sdk, []byte("export {};"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewClaudeAdapter(path, sdk, path, map[string]string{"account": home})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := a.Start(ctx, r, func(AdapterEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "steer", "follow up"); err != nil {
		t.Fatal(err)
	}
	if err := p.Control(ctx, "interrupt", ""); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	_ = p.Wait()
}
