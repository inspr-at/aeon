// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/inspr-at/aeon/internal/agentd"
)

var localToolRE = regexp.MustCompile(`^[A-Za-z0-9_.*:-]+$`)

func validateLocalClaudeConfig(o runAgentOptions) error {
	modes := map[string]bool{"acceptEdits": true, "auto": true, "bypassPermissions": true, "manual": true, "dontAsk": true, "plan": true}
	if !modes[o.PermissionMode] {
		return usagef("invalid --claude-permission-mode")
	}
	if o.PermissionMode == "bypassPermissions" && !o.UnsafeBypass {
		return usagef("bypassPermissions requires --unsafe-allow-bypass-permissions")
	}
	tools := strings.Split(strings.TrimSpace(o.AllowedTools), ",")
	if len(tools) < 1 || len(tools) > 64 {
		return usagef("--claude-allowed-tools must contain 1-64 tool names")
	}
	for _, tool := range tools {
		if !localToolRE.MatchString(strings.TrimSpace(tool)) {
			return usagef("invalid --claude-allowed-tools value")
		}
	}
	return nil
}

type localClaudeAdapter struct {
	opts           runAgentOptions
	homes          map[string]string
	stdin          io.Reader
	stdout, stderr io.Writer
	projectID      string
	idle, maxRun   time.Duration
}

func (*localClaudeAdapter) Name() string { return agentd.Claude }

func (a *localClaudeAdapter) Probe(ctx context.Context, key string) bool {
	if _, ok := a.homes[key]; !ok {
		return false
	}
	if strings.TrimSpace(a.opts.Exec) != "claude" {
		return true
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, "claude", "auth", "status", "--json")
	if home := a.homes[key]; home != "" {
		cmd.Env = localRunnerEnv(home)
	}
	raw, err := cmd.Output()
	if err != nil || len(raw) > 64<<10 {
		return false
	}
	var state struct {
		LoggedIn bool `json:"loggedIn"`
	}
	return json.Unmarshal(raw, &state) == nil && state.LoggedIn
}

func (a *localClaudeAdapter) Start(_ context.Context, request agentd.StartRequest, _ func(agentd.AdapterEvent)) (agentd.Process, error) {
	if !a.Probe(context.Background(), request.AccountKey) {
		return nil, errors.New("Claude account unavailable")
	}
	if !a.opts.Yes {
		fmt.Fprintf(a.stdout, "Run %s in %s? [y/N] ", request.Run.ID, request.Workspace)
		line, err := bufio.NewReader(a.stdin).ReadString('\n')
		if err != nil || (strings.TrimSpace(line) != "y" && strings.TrimSpace(line) != "yes") {
			return nil, errors.New("run declined")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.maxRun)
	var command *exec.Cmd
	if strings.TrimSpace(a.opts.Exec) == "claude" {
		argv := []string{"-p", "--verbose", "--output-format", "stream-json",
			"--permission-mode", a.opts.PermissionMode, "--tools", a.opts.AllowedTools, "--allowedTools", a.opts.AllowedTools}
		if a.opts.PermissionMode == "bypassPermissions" {
			argv = append(argv, "--allow-dangerously-skip-permissions")
		}
		command = exec.CommandContext(ctx, "claude", argv...)
	} else {
		command = exec.CommandContext(ctx, "sh", "-c", a.opts.Exec)
	}
	command.Dir = request.Workspace
	command.Stdin = strings.NewReader(request.Prompt)
	command.Env = localRunnerEnv(a.homes[request.AccountKey])
	command.Env = append(command.Env, "PAIMOS_RUN_ID="+request.Run.ID, "PAIMOS_PROJECT_ID="+a.projectID)
	var promptFile string
	if strings.TrimSpace(a.opts.Exec) != "claude" {
		file, err := os.CreateTemp(request.StateRoot, "paimos-prompt-*")
		if err != nil {
			cancel()
			return nil, err
		}
		promptFile = file.Name()
		if _, err = file.WriteString(request.Prompt); err != nil {
			_ = file.Close()
			_ = os.Remove(promptFile)
			cancel()
			return nil, err
		}
		if err = file.Close(); err != nil {
			_ = os.Remove(promptFile)
			cancel()
			return nil, err
		}
		command.Env = append(command.Env, "PAIMOS_PROMPT_FILE="+promptFile)
	}
	proc := &localRunnerProcess{cmd: command, cancel: cancel, done: make(chan struct{}), promptFile: promptFile,
		testExec: a.opts.TestExec, workspace: request.Workspace, attachLogs: a.opts.AttachLogs,
		stdout: a.stdout, stderr: a.stderr}
	proc.lastActivity.Store(time.Now().UnixNano())
	command.Stdout = &activityWriter{target: a.stdout, log: &proc.log, capture: a.opts.AttachLogs, activity: &proc.lastActivity}
	command.Stderr = &activityWriter{target: a.stderr, log: &proc.log, capture: a.opts.AttachLogs, activity: &proc.lastActivity}
	if err := command.Start(); err != nil {
		if promptFile != "" {
			_ = os.Remove(promptFile)
		}
		cancel()
		return nil, err
	}
	if a.idle > 0 {
		go proc.watchIdle(a.idle)
	}
	return proc, nil
}

func localRunnerEnv(home string) []string {
	environment := os.Environ()
	if home == "" {
		return environment
	}
	filtered := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, "CLAUDE_CONFIG_DIR=") {
			filtered = append(filtered, item)
		}
	}
	return append(filtered, "CLAUDE_CONFIG_DIR="+home)
}

type boundedRunnerLog struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *boundedRunnerLog) write(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := (64 << 10) - b.data.Len()
	if remaining > len(p) {
		remaining = len(p)
	}
	if remaining > 0 {
		_, _ = b.data.Write(p[:remaining])
	}
}

func (b *boundedRunnerLog) string() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

type activityWriter struct {
	target   io.Writer
	log      *boundedRunnerLog
	capture  bool
	activity *atomic.Int64
}

func (w *activityWriter) Write(p []byte) (int, error) {
	w.activity.Store(time.Now().UnixNano())
	if w.capture {
		w.log.write(p)
	}
	return w.target.Write(p)
}

type localRunnerProcess struct {
	cmd                             *exec.Cmd
	cancel                          context.CancelFunc
	done                            chan struct{}
	lastActivity                    atomic.Int64
	log                             boundedRunnerLog
	promptFile, testExec, workspace string
	attachLogs                      bool
	stdout, stderr                  io.Writer
}

func (p *localRunnerProcess) PID() int { return p.cmd.Process.Pid }

func (p *localRunnerProcess) Wait() error {
	defer close(p.done)
	defer p.cancel()
	defer func() {
		if p.promptFile != "" {
			_ = os.Remove(p.promptFile)
		}
	}()
	if err := p.cmd.Wait(); err != nil {
		return err
	}
	if strings.TrimSpace(p.testExec) != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", p.testExec)
		cmd.Dir = p.workspace
		cmd.Stdout, cmd.Stderr = p.stdout, p.stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("tests failed: %w", err)
		}
	}
	return nil
}

func (p *localRunnerProcess) Evidence() string {
	if p.attachLogs {
		if output := p.log.string(); output != "" {
			return output
		}
	}
	return "Local Claude Code run completed; changes remain for review."
}

func (p *localRunnerProcess) Control(_ context.Context, _, _ string) error {
	return agentd.ErrUnsupported
}

func (p *localRunnerProcess) Stop(_ context.Context) error {
	p.cancel()
	return nil
}

func (p *localRunnerProcess) watchIdle(timeout time.Duration) {
	interval := timeout / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			if time.Since(time.Unix(0, p.lastActivity.Load())) > timeout {
				p.cancel()
				return
			}
		}
	}
}

var _ agentd.Adapter = (*localClaudeAdapter)(nil)
var _ agentd.AccountProber = (*localClaudeAdapter)(nil)
var _ agentd.EvidenceProcess = (*localRunnerProcess)(nil)
