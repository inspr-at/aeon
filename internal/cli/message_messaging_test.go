// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runMessagingCLI exercises the exported coordinator entry point.
func runMessagingCLI(args []string, input string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := RunMessaging(append([]string{"paimos"}, args...), strings.NewReader(input), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestMessagePrivateFileAndErrorRedaction(t *testing.T) {
	isolate(t)
	keyPath := filepath.Join(t.TempDir(), "routine-key")
	const fixtureKey = "synthetic-routine-fixture-value"
	if err := os.WriteFile(keyPath, []byte(fixtureKey), 0600); err != nil {
		t.Fatal(err)
	}
	rt := &runtime{stdin: strings.NewReader("fixture-thread\n")}
	if ref, err := rt.readMessagingPrivateFile("-", false); err != nil || ref != "fixture-thread" {
		t.Fatal("stdin reference", err)
	}
	if key, err := rt.readMessagingPrivateFile(keyPath, true); err != nil || key != fixtureKey {
		t.Fatal("private key read failed")
	}
	if err := os.Chmod(keyPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.readMessagingPrivateFile(keyPath, true); err == nil {
		t.Fatal("publicly readable key accepted")
	}
	if err := os.Chmod(keyPath, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(keyPath, link); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.readMessagingPrivateFile(link, true); err == nil {
		t.Fatal("symlink key accepted")
	}
	hardlinkSource := filepath.Join(t.TempDir(), "linked-key")
	if err := os.WriteFile(hardlinkSource, []byte(fixtureKey), 0600); err != nil {
		t.Fatal(err)
	}
	hardlink := filepath.Join(t.TempDir(), "hardlink")
	if err := os.Link(hardlinkSource, hardlink); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.readMessagingPrivateFile(hardlinkSource, true); err == nil {
		t.Fatal("multiply linked key accepted")
	}
	rt.stdin = strings.NewReader(fixtureKey + "\n")
	if key, err := rt.readMessagingPrivateFile("-", true); err != nil || key != fixtureKey {
		t.Fatal("stdin target key failed")
	}
	if _, err := rt.readMessagingPrivateFile(filepath.Join(t.TempDir(), fixtureKey), true); err == nil || strings.Contains(err.Error(), fixtureKey) {
		t.Fatal("private pathname in error")
	}
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/kinds":
			fmt.Fprint(w, `{"items":[{"id":"00000000-0000-4000-8000-000000000001","slug":"project"}]}`)
		case "/api/nodes":
			fmt.Fprint(w, `{"items":[{"id":"00000000-0000-4000-8000-000000000002","kind_id":"00000000-0000-4000-8000-000000000001","key":"AEON-1","title":"AEON","fields":{"project_key":"AEON"}}]}`)
		default:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("missing target body")
			}
			received = body["target_secret"] == fixtureKey
			w.WriteHeader(500)
			fmt.Fprint(w, fixtureKey)
		}
	}))
	defer srv.Close()
	t.Setenv("PAIMOS_URL", srv.URL)
	t.Setenv("PAIMOS_API_KEY", "fixture-api-key")
	code, out, stderr := runMessagingCLI([]string{"--config", filepath.Join(t.TempDir(), "missing"), "message", "target", "set", "--project", "AEON", "--address", "grok_bot:worker", "--adapter", "grok_bot_routine", "--kind", "https_webhook", "--target-ref-file", "-", "--target-key-file", keyPath}, "https://fixture.invalid/hook")
	if code != 1 || !received || strings.Contains(out+stderr, fixtureKey) {
		t.Fatalf("registration failed safely=%v code=%d", received, code)
	}
}
func TestMessagePrivatePostDoesNotRedirect(t *testing.T) {
	isolate(t)
	var followed bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer origin.Close()
	t.Setenv("PAIMOS_URL", origin.URL)
	t.Setenv("PAIMOS_API_KEY", "fixture-api-key")
	rt := &runtime{configPath: filepath.Join(t.TempDir(), "missing"), program: "paimos"}
	if err := rt.privateMessagingPost("/api/message-targets", map[string]string{"target_secret": "fixture"}, nil); err == nil {
		t.Fatal("redirect accepted")
	}
	if followed {
		t.Fatal("private target followed redirect")
	}
}

func TestMessagingEntryPointPreservesGlobalsAndTree(t *testing.T) {
	isolate(t)
	for _, args := range [][]string{{"--help"}, {"--version"}, {"tell", "--help"}, {"listen", "--help"}, {"message", "target", "set", "--help"}, {"issue", "--help"}} {
		code, out, errOut := runMessagingCLI(args, "")
		if code != 0 || out == "" || errOut != "" {
			t.Fatalf("entry point %v code %d", args, code)
		}
	}
	for _, tc := range unsupportedCompat {
		if len(tc.Args) == 0 {
			continue
		}
		code, _, errOut := runMessagingCLI(tc.Args, "")
		if code != 3 || !strings.Contains(errOut, tc.Reason) {
			t.Fatalf("unsupported %s changed", tc.Name)
		}
	}
}
