// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeRunTools struct {
	comments  []string
	status    string
	criterion string
	evidence  string
	approval  string
	reply     string
}

func (f *fakeRunTools) WorkOrder(context.Context, string) (WorkOrder, error) {
	now := time.Now()
	return WorkOrder{Revision: 4, Criteria: []WorkCriterion{{ID: "11111111-1111-1111-1111-111111111111", CheckedAt: &now}}}, nil
}
func (f *fakeRunTools) Comment(_ context.Context, id, body string) error {
	f.comments = append(f.comments, id+":"+body)
	return nil
}
func (f *fakeRunTools) SetWorkStatus(_ context.Context, id string, revision int64, status string) (WorkOrder, error) {
	f.status = id + ":" + status
	return WorkOrder{NodeID: id, Status: status, Revision: revision + 1}, nil
}
func (f *fakeRunTools) CheckCriterion(_ context.Context, id, criterion string, checked bool) error {
	f.criterion = id + ":" + criterion
	return nil
}
func (f *fakeRunTools) Evidence(_ context.Context, id, run, criterion, ref string) error {
	f.evidence = id + ":" + run + ":" + criterion + ":" + ref
	return nil
}
func (f *fakeRunTools) RequestApproval(_ context.Context, run, scope, rationale, expiry string) error {
	f.approval = run + ":" + scope
	return nil
}
func (f *fakeRunTools) ReplyInbox(_ context.Context, id, recipient, body, key string) error {
	f.reply = id + ":" + body
	return nil
}

type headerTransport struct{ token string }

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestManagedToolsRunBinding(t *testing.T) {
	ctx := t.Context()
	f := &fakeRunTools{}
	active := true
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow synthetic test token
	doneRequested := false
	host, err := startManagedTools(token, toolBinding{api: f, workOrderID: "order-a", runID: "run-a", workspace: t.TempDir(), branch: "aeon/run-a", active: func() bool { return active }, requestDone: func() { doneRequested = true }, replySender: func(id string) (string, bool) {
		if id == "33333333-3333-3333-3333-333333333333" {
			return "44444444-4444-4444-4444-444444444444", true
		}
		return "", false
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	bad := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	if session, err := bad.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: host.tools.URL, HTTPClient: &http.Client{Transport: headerTransport{token: "wrong"}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil); err == nil {
		_ = session.Close()
		t.Fatal("wrong run credential accepted")
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: host.tools.URL, HTTPClient: &http.Client{Transport: headerTransport{token: token}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(name string, args map[string]any, wantError bool) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil && !wantError {
			t.Fatalf("%s: %v", name, err)
		}
		if err == nil && result.IsError != wantError {
			t.Fatalf("%s error=%v want %v", name, result.IsError, wantError)
		}
	}
	call("aeon_comment", map[string]any{"body": "progress"}, false)
	call("aeon_status", map[string]any{"status": "blocked", "expected_revision": int64(4)}, false)
	call("aeon_status", map[string]any{"status": "done", "expected_revision": int64(4)}, false)
	call("aeon_evidence", map[string]any{"criterion_id": "22222222-2222-2222-2222-222222222222", "reference": "wrong"}, true)
	call("aeon_evidence", map[string]any{"criterion_id": "11111111-1111-1111-1111-111111111111", "reference": "tests pass"}, false)
	call("aeon_reply", map[string]any{"message_id": "22222222-2222-2222-2222-222222222222", "body": "wrong", "idempotency_key": "reply-1"}, true)
	call("aeon_reply", map[string]any{"message_id": "33333333-3333-3333-3333-333333333333", "body": "received", "idempotency_key": "reply-2"}, false)
	if len(f.comments) != 1 || f.comments[0] != "order-a:progress" || f.status != "order-a:blocked" || !doneRequested || f.evidence != "order-a:run-a:11111111-1111-1111-1111-111111111111:tests pass" {
		t.Fatalf("wrong binding: %+v", f)
	}
	active = false
	call("aeon_comment", map[string]any{"body": "late"}, true)
	if len(f.comments) != 1 {
		t.Fatal("expired run wrote a comment")
	}
}

func TestRunCredentialIsFenced(t *testing.T) {
	r := NewRemote("https://aeon.example.invalid", "daemon-key")
	a := r.runCredential("tenant", "agent", "run-a", "generation-a")
	for _, other := range []string{
		r.runCredential("tenant", "agent", "run-b", "generation-a"),
		r.runCredential("tenant", "agent", "run-a", "generation-b"),
		r.runCredential("tenant-b", "agent", "run-a", "generation-a"),
	} {
		if a == other || len(other) != 64 {
			t.Fatal("run credential was not fenced")
		}
	}
	if strings.Contains(a, "daemon-key") {
		t.Fatal("daemon key leaked")
	}
}

type doneToolAPI struct {
	*fakeAPI
	mu   sync.Mutex
	done bool
}

func (*doneToolAPI) runCredential(string, string, string, string) string {
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
func (*doneToolAPI) WorkOrder(context.Context, string) (WorkOrder, error) {
	now := time.Now()
	return WorkOrder{NodeID: "order", Status: "running", Revision: 4, Criteria: []WorkCriterion{{ID: "11111111-1111-1111-1111-111111111111", CheckedAt: &now}}}, nil
}
func (*doneToolAPI) Comment(context.Context, string, string) error { return nil }
func (a *doneToolAPI) SetWorkStatus(_ context.Context, id string, rev int64, status string) (WorkOrder, error) {
	a.fakeAPI.mu.Lock()
	defer a.fakeAPI.mu.Unlock()
	if len(a.fakeAPI.reports) == 0 || a.fakeAPI.reports[len(a.fakeAPI.reports)-1].Status != "completed" {
		return WorkOrder{}, errors.New("run still active")
	}
	a.mu.Lock()
	a.done = id == "order" && rev == 4 && status == "done"
	a.mu.Unlock()
	return WorkOrder{NodeID: id, Status: status, Revision: rev + 1}, nil
}
func (*doneToolAPI) CheckCriterion(context.Context, string, string, bool) error     { return nil }
func (*doneToolAPI) Evidence(context.Context, string, string, string, string) error { return nil }
func (*doneToolAPI) RequestApproval(context.Context, string, string, string, string) error {
	return nil
}
func (*doneToolAPI) ReplyInbox(context.Context, string, string, string, string) error { return nil }

func TestDoneRequestAppliesAfterRunFinishes(t *testing.T) {
	s, base, process := testSupervisor(t)
	defer s.Close(context.Background())
	api := &doneToolAPI{fakeAPI: base}
	s.api = api
	if err := s.PollOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	entry := s.runs["run"]
	if entry.tools == nil {
		t.Fatal("run tools were not started")
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: entry.tools.tools.URL, HTTPClient: &http.Client{Transport: headerTransport{token: entry.tools.tools.Token}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "aeon_status", Arguments: map[string]any{"status": "done", "expected_revision": int64(4)}})
	if err != nil || result.IsError {
		t.Fatalf("done request: %v %+v", err, result)
	}
	api.mu.Lock()
	early := api.done
	api.mu.Unlock()
	if early {
		t.Fatal("done applied while run was active")
	}
	if err := process.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	entry.mu.Lock()
	done := entry.monitorDone
	entry.mu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish")
	}
	api.mu.Lock()
	finished := api.done
	api.mu.Unlock()
	if !finished {
		t.Fatal("requested done transition was not applied after completion")
	}
}

func TestTerminalPolicyAndBranchFence(t *testing.T) {
	for _, in := range []terminalArgs{
		{Command: "sh", Args: []string{"-c", "git push"}},
		{Command: "git", Args: []string{"push"}},
		{Command: "git", Args: []string{"commit", "--no-verify", "-m", "bad"}},
		{Command: "go", Args: []string{"test", "-exec=sh", "./..."}},
		{Command: "go", Args: []string{"test", "../..."}},
		{Command: "npm", Args: []string{"install"}},
	} {
		if allowedTerminal(in.Command, in.Args) {
			t.Fatalf("accepted %+v", in)
		}
	}
	workspace := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	// The allowlist above is platform-independent; running it needs the macOS sandbox.
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox")
	}
	if _, err := runTerminal(t.Context(), workspace, "aeon/run-a", terminalArgs{Command: "git", Args: []string{"commit", "-m", "message"}}); err == nil || !strings.Contains(err.Error(), "branch") {
		t.Fatalf("branch fence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "safe.go"), []byte("package safe\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runTerminal(t.Context(), workspace, "main", terminalArgs{Command: "git", Args: []string{"add", "--", ".env"}}); err == nil {
		t.Fatal("secret file accepted")
	}
	if _, err := runTerminal(t.Context(), workspace, "main", terminalArgs{Command: "git", Args: []string{"add", "--", "safe.go"}}); err == nil {
		t.Fatal("main branch accepted")
	}
	switchBranch := exec.Command("git", "switch", "-c", "aeon/run-a")
	switchBranch.Dir = workspace
	if out, err := switchBranch.CombinedOutput(); err != nil {
		t.Fatalf("switch: %v %s", err, out)
	}
	if _, err := runTerminal(t.Context(), workspace, "aeon/run-a", terminalArgs{Command: "git", Args: []string{"add", "--", ".env"}}); err == nil {
		t.Fatal("secret stage accepted")
	}
	if out, err := runTerminal(t.Context(), workspace, "aeon/run-a", terminalArgs{Command: "git", Args: []string{"add", "--", "safe.go"}}); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if _, err := runTerminal(t.Context(), workspace, "aeon/run-a", terminalArgs{Command: "git", Args: []string{"commit", "-m", "AEON test"}}); err != nil {
		t.Fatal(err)
	}
	if sha, err := runTerminal(t.Context(), workspace, "aeon/run-a", terminalArgs{Command: "git", Args: []string{"rev-parse", "HEAD"}}); err != nil || len(strings.TrimSpace(sha)) != 40 {
		t.Fatalf("commit sha: %q %v", sha, err)
	}
	if _, err := runTerminal(t.Context(), workspace, "main", terminalArgs{Command: "git", Args: []string{"push"}}); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("push error: %v", err)
	}
}

func TestTerminalSandboxesTestProcessNetwork(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox")
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.invalid/sandboxtest\n\ngo 1.25\n"), 0600); err != nil {
		t.Fatal(err)
	}
	const source = `package sandboxtest
import ("net"; "testing"; "time")
func TestNetworkFence(t *testing.T) {
  l,err:=net.Listen("tcp","127.0.0.1:0"); if err!=nil { t.Fatalf("loopback bind: %v",err) }; l.Close()
  c,err:=net.DialTimeout("tcp","1.1.1.1:443",time.Second)
  if c!=nil { c.Close(); t.Fatal("external network escaped sandbox") }
  if err==nil { t.Fatal("external network escaped sandbox") }
}`
	if err := os.WriteFile(filepath.Join(workspace, "network_test.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := runTerminal(t.Context(), workspace, "", terminalArgs{Command: "go", Args: []string{"test", "./..."}}); err != nil {
		t.Fatalf("sandboxed test: %v\n%s", err, out)
	}
}

func TestTerminalSandboxesTestProcessFiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(home, "work")
	for _, path := range []string{workspace, filepath.Join(home, ".ssh")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".ssh", "fixture"), []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "ordinary-file"), []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.invalid/filetest\n\ngo 1.25\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "sample.go"), []byte("package filetest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, ".zshrc-like")
	malicious := fmt.Sprintf(`package filetest
import ("os"; "testing")
func TestOutsideWrite(t *testing.T) {
  if err := os.WriteFile(%q, []byte("persist"), 0600); err != nil { t.Fatal(err) }
}`, outside)
	testFile := filepath.Join(workspace, "file_test.go")
	if err := os.WriteFile(testFile, []byte(malicious), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := runTerminal(t.Context(), workspace, "", terminalArgs{Command: "go", Args: []string{"test", "./..."}}); err == nil || !strings.Contains(out, "TestOutsideWrite") {
		t.Fatalf("outside write was not rejected by the test process: %v\n%s", err, out)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside path exists after denied write: %v", err)
	}
	allowed := fmt.Sprintf(`package filetest
import ("net"; "os"; "testing"; "time")
func TestSandbox(t *testing.T) {
  if err := os.WriteFile("inside", []byte("ok"), 0600); err != nil { t.Fatalf("workspace write: %%v", err) }
  if err := os.WriteFile("temp-path", []byte(os.Getenv("TMPDIR")), 0600); err != nil { t.Fatal(err) }
  if _,err := os.ReadFile(%q); err == nil { t.Fatal("read escaped sandbox") }
  if _,err := os.ReadFile(%q); err == nil { t.Fatal("home read escaped sandbox") }
  if err := os.WriteFile(%q, []byte("persist"), 0600); err == nil { t.Fatal("write escaped sandbox") }
  if err := os.WriteFile("escape/symlink-write", []byte("persist"), 0600); err == nil { t.Fatal("symlink write escaped sandbox") }
  c,err := net.DialTimeout("tcp", "1.1.1.1:443", time.Second)
  if c != nil { c.Close(); t.Fatal("network escaped sandbox") }
  if err == nil { t.Fatal("network escaped sandbox") }
}`, filepath.Join(home, ".ssh", "fixture"), filepath.Join(home, "ordinary-file"), outside)
	if err := os.WriteFile(testFile, []byte(allowed), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := runTerminal(t.Context(), workspace, "", terminalArgs{Command: "go", Args: []string{"test", "./..."}}); err != nil {
		t.Fatalf("allowed sandboxed test: %v\n%s", err, out)
	}
	for _, verb := range []string{"build", "vet"} {
		if out, err := runTerminal(t.Context(), workspace, "", terminalArgs{Command: "go", Args: []string{verb, "./..."}}); err != nil {
			t.Fatalf("sandboxed go %s: %v\n%s", verb, err, out)
		}
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "inside")); err != nil || string(content) != "ok" {
		t.Fatalf("workspace output: %q %v", content, err)
	}
	if tempPath, err := os.ReadFile(filepath.Join(workspace, "temp-path")); err != nil {
		t.Fatal(err)
	} else if _, err := os.Stat(string(tempPath)); !os.IsNotExist(err) {
		t.Fatalf("per-run temp directory was not removed: %v", err)
	}
	web := filepath.Join(workspace, "web")
	if err := os.Mkdir(web, 0700); err != nil {
		t.Fatal(err)
	}
	const packageJSON = `{"scripts":{"test":"node -e 'require(\"fs\").writeFileSync(\"npm-inside\",\"ok\")'","build":"node -e 'process.exit(0)'"}}`
	if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(packageJSON), 0600); err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"test", "build"} {
		if out, err := runTerminal(t.Context(), workspace, "", terminalArgs{Command: "npm", Args: []string{"run", script}, Directory: "web"}); err != nil {
			t.Fatalf("npm run %s: %v\n%s", script, err, out)
		}
	}
	if content, err := os.ReadFile(filepath.Join(web, "npm-inside")); err != nil || string(content) != "ok" {
		t.Fatalf("npm workspace output: %q %v", content, err)
	}
}
