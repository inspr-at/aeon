// SPDX-License-Identifier: AGPL-3.0-only
//go:build darwin

package agentd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestGrokAssetsStayPinned(t *testing.T) {
	config, err := grokAssets.ReadFile("grokassets/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := grokAssets.ReadFile("grokassets/conversation.txt")
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(config) != grokConfigSHA256 || sha256Hex(profile) != grokProfileSHA256 {
		t.Fatal("Grok isolation assets changed")
	}
	if _, err := NewGrokAdapter(map[string]GrokBinding{"account": {Variant: "invalid"}}).Start(context.Background(), StartRequest{
		Profile: Profile{Harness: Grok, Model: grokModel, Effort: grokEffort}, AccountKey: "account"}, func(AdapterEvent) {}); err == nil {
		t.Fatal("unknown native variant accepted")
	}
}

func TestGrokNotificationsRejectToolsAndBoundAnswer(t *testing.T) {
	p := &grokProcess{sessionID: "session"}
	valid := map[string]any{"method": "session/update", "params": map[string]any{"sessionId": "session", "update": map[string]any{
		"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "answer"}}}}
	raw, _ := json.Marshal(valid)
	p.onEvent(raw)
	if p.violation.Load() || p.Evidence() != "answer" {
		t.Fatal("bounded answer event rejected")
	}
	p.onEvent([]byte(`{"method":"session/request_permission","params":{"sessionId":"session"}}`))
	if !p.violation.Load() {
		t.Fatal("tool permission request accepted")
	}
	p2 := &grokProcess{sessionID: "session"}
	valid["params"] = map[string]any{"sessionId": "other", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "wrong"}}}
	raw, _ = json.Marshal(valid)
	p2.onEvent(raw)
	if !p2.violation.Load() {
		t.Fatal("foreign session update accepted")
	}
}

func TestGrokProxyRejectsOtherTargets(t *testing.T) {
	p, err := startGrokProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()
	conn, err := net.DialTimeout("tcp", p.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, err = conn.Write([]byte("CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil || !bytes.Contains(line, []byte("403")) {
		t.Fatalf("proxy did not reject foreign target: %q %v", line, err)
	}
}
