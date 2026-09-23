// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/client"
)

// Kept while the coordinator replaces the legacy cmdHarness/session-start
// branches in compat_cmds.go with the constructors below.
const (
	reasonHarness = "harness command wiring awaits coordinator reconciliation"
	reasonBundle  = "session full-bundle wiring awaits coordinator reconciliation"
)

// cmdHarnessV2 is the complete P5.3 command tree. The coordinator replaces
// the legacy cmdHarness stub in compat_cmds.go with this constructor.
func (rt *runtime) cmdHarnessV2() *Command {
	return &Command{Name: "harness", Short: "Manage durable harness generations", Use: "harness <command>", subs: []*Command{
		rt.harnessRegister(), rt.harnessRead("list"), rt.harnessRead("status"), rt.harnessRead("orchestrator"), rt.harnessBind(), rt.harnessWorker("heartbeat"), rt.harnessWorker("yield"), rt.harnessWorker("drain"), rt.harnessWorker("complete-delivery"), rt.harnessControl("interrupt"), rt.harnessControl("stop"), rt.harnessControl("complete-control"), rt.harnessWorker("mark-stopped"),
	}}
}

func harnessSecret(path, label string) (string, error) {
	if path == "" {
		return "", usagef("--%s is required", label)
	}
	if path == "-" {
		return "", usagef("--%s must be a protected file", label)
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0o077 != 0 {
		return "", usagef("--%s must be a private regular file", label)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(raw))
	if len(value) < 16 || strings.ContainsAny(value, "\r\n") {
		return "", usagef("--%s must contain one value", label)
	}
	return value, nil
}

// harnessDo refuses redirects, so a private registration reference or worker
// lease cannot follow a server redirect to another origin.
func (rt *runtime) harnessDo(method, path, lease string, body, dest any) error {
	c, err := rt.api()
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.BaseURL+path, reader)
	if err != nil {
		return rt.fail(err, "")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if lease != "" {
		req.Header.Set("X-Aeon-Worker-Lease", lease)
	}
	hc := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("harness request redirect refused") }}
	res, err := hc.Do(req)
	if err != nil {
		return rt.fail(err, "")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return rt.fail(err, "")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var v struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &v)
		if v.Error == "" {
			v.Error = http.StatusText(res.StatusCode)
		}
		return rt.fail(&client.StatusError{Status: res.StatusCode, Message: v.Error}, "")
	}
	if dest != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err = json.Unmarshal(raw, dest); err != nil {
			return rt.fail(fmt.Errorf("decode harness response: %w", err), "")
		}
	}
	return nil
}
func (rt *runtime) harnessProject(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", usagef("--project is required")
	}
	n, err := rt.projectNode(ref)
	return n.ID, err
}
func (rt *runtime) printHarness(v any) error {
	if rt.jsonOut {
		return rt.printJSON(v)
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(rt.stdout, string(raw))
	return nil
}
func harnessPath(projectID, sessionID string) string {
	path := "/api/projects/" + projectID + "/harness-sessions"
	if sessionID != "" {
		path += "/" + sessionID
	}
	return path
}

func (rt *runtime) harnessRegister() *Command {
	var project, agent, harness, host, refFile, leaseFile, management, role, parent, ticket, shape, runID, orderID string
	var caps []string
	return &Command{Name: "register", Short: "Register one public harness generation", Use: "harness register --project KEY --agent NAME --harness KIND --host HOST --harness-session-file PATH --worker-lease-file PATH", addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 'p', "project key")
		fs.string(&agent, "agent", 0, "agent principal name")
		fs.string(&harness, "harness", 0, "adapter family")
		fs.string(&host, "host", 0, "non-secret host label")
		fs.string(&refFile, "harness-session-file", 0, "private external reference file")
		fs.string(&leaseFile, "worker-lease-file", 0, "private generation lease file")
		fs.string(&management, "management", 0, "managed or unmanaged")
		fs.string(&role, "role", 0, "worker or coordinator")
		fs.string(&parent, "parent-session", 0, "parent public session UUID")
		fs.string(&ticket, "ticket", 0, "ticket node key")
		fs.string(&shape, "work-shape", 0, "ship or scout")
		fs.string(&runID, "run-id", 0, "Aeon agent run UUID")
		fs.string(&orderID, "work-order-id", 0, "Aeon work order UUID")
		fs.strings(&caps, "capability", "comma-separated advertised capabilities")
	}, run: func([]string) error {
		if !agentNameRE.MatchString(agent) || !modelHarnesses[harness] || strings.TrimSpace(host) == "" {
			return usagef("--agent, --harness and --host are required")
		}
		if management == "" {
			management = "managed"
		}
		if role == "" {
			role = "worker"
		}
		if management != "managed" && management != "unmanaged" || role != "worker" && role != "coordinator" {
			return usagef("invalid management or role")
		}
		projectID, err := rt.harnessProject(project)
		if err != nil {
			return err
		}
		me, err := rt.caller()
		if err != nil {
			return err
		}
		if me.Principal.Name != agent {
			return usagef("--agent must name the authenticated agent")
		}
		ref, err := harnessSecret(refFile, "harness-session-file")
		if err != nil {
			return err
		}
		lease, err := harnessSecret(leaseFile, "worker-lease-file")
		if err != nil {
			return err
		}
		if len(lease) < 32 {
			return usagef("worker lease must contain at least 32 characters")
		}
		var ticketID, parentID, run, order *string
		if parent != "" {
			if !validUUID(parent) {
				return usagef("invalid parent session")
			}
			parentID = &parent
		}
		if ticket != "" {
			n, e := rt.nodeByKey(ticket)
			if e != nil {
				return e
			}
			ticketID = &n.ID
			if shape != "ship" && shape != "scout" {
				return usagef("--work-shape must be ship or scout")
			}
		} else if shape != "" {
			return usagef("--work-shape requires --ticket")
		}
		if runID != "" {
			if !validUUID(runID) {
				return usagef("invalid run id")
			}
			run = &runID
		}
		if orderID != "" {
			if !validUUID(orderID) {
				return usagef("invalid work order id")
			}
			order = &orderID
		}
		var out any
		err = rt.harnessDo(http.MethodPost, harnessPath(projectID, ""), "", map[string]any{"agent_principal_id": me.Principal.ID, "harness": harness, "host": host, "harness_session_ref": ref, "worker_lease": lease, "management_mode": management, "role": role, "parent_harness_session_id": parentID, "ticket_node_id": ticketID, "work_shape": shape, "work_order_id": order, "run_id": run, "advertised_capabilities": caps}, &out)
		if err != nil {
			return err
		}
		return rt.printHarness(out)
	}}
}
func (rt *runtime) harnessRead(kind string) *Command {
	var project, session string
	return &Command{Name: kind, Short: "Read public harness state", Use: "harness " + kind + " --project KEY", addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 'p', "project key")
		if kind == "status" {
			fs.string(&session, "session", 0, "public session UUID")
		}
	}, run: func([]string) error {
		id, err := rt.harnessProject(project)
		if err != nil {
			return err
		}
		if kind == "status" && !validUUID(session) {
			return usagef("--session UUID is required")
		}
		path := harnessPath(id, session)
		if kind == "orchestrator" {
			path += "/orchestrator"
		}
		var out any
		if err = rt.harnessDo(http.MethodGet, path, "", nil, &out); err != nil {
			return err
		}
		return rt.printHarness(out)
	}}
}
func (rt *runtime) harnessBind() *Command {
	var project, session, parent, ticket, shape string
	var revision int
	return &Command{Name: "bind", Short: "Compare-and-set hierarchy and ticket binding", Use: "harness bind --project KEY --session UUID --revision N --parent-session UUID --ticket KEY --work-shape ship|scout", addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 'p', "project key")
		fs.string(&session, "session", 0, "public session UUID")
		fs.int(&revision, "revision", "current revision")
		fs.string(&parent, "parent-session", 0, "parent session or empty")
		fs.string(&ticket, "ticket", 0, "ticket key or empty")
		fs.string(&shape, "work-shape", 0, "ship, scout or unknown")
	}, run: func([]string) error {
		id, err := rt.harnessProject(project)
		if err != nil {
			return err
		}
		if !validUUID(session) || revision < 1 {
			return usagef("--session and positive --revision are required")
		}
		var parentID, ticketID *string
		if parent != "" {
			if !validUUID(parent) {
				return usagef("invalid parent session")
			}
			parentID = &parent
		}
		if ticket != "" {
			n, e := rt.nodeByKey(ticket)
			if e != nil {
				return e
			}
			ticketID = &n.ID
			if shape != "ship" && shape != "scout" {
				return usagef("bound ticket needs ship or scout")
			}
		} else if shape != "unknown" {
			return usagef("detached ticket needs unknown work shape")
		}
		var out any
		err = rt.harnessDo(http.MethodPatch, harnessPath(id, session)+"/binding", "", map[string]any{"expected_revision": revision, "parent_harness_session_id": parentID, "ticket_node_id": ticketID, "work_shape": shape}, &out)
		if err != nil {
			return err
		}
		return rt.printHarness(out)
	}}
}
func (rt *runtime) harnessWorker(kind string) *Command {
	var project, session, agent, leaseFile, phase, activity, deliveryID, level, reason string
	var sequence, cursor int
	return &Command{Name: kind, Short: "Act as the attributed harness worker", Use: "harness " + kind + " --project KEY --session UUID --agent NAME --worker-lease-file PATH", addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 'p', "project key")
		fs.string(&session, "session", 0, "public session UUID")
		fs.string(&agent, "agent", 0, "attributed agent name")
		fs.string(&leaseFile, "worker-lease-file", 0, "private generation lease file")
		switch kind {
		case "heartbeat":
			fs.string(&phase, "phase", 0, "starting, working, yielded or stopping")
			fs.string(&activity, "activity", 0, "unknown, busy or idle")
			fs.int(&sequence, "activity-sequence", "monotonic sequence")
		case "complete-delivery":
			fs.string(&deliveryID, "delivery-id", 0, "leased delivery UUID")
			fs.int(&cursor, "cursor", "sent-event cursor")
			fs.string(&level, "effective-level", 0, "simple")
		case "mark-stopped":
			fs.string(&reason, "reason", 0, "stop reason")
		}
	}, run: func([]string) error {
		id, err := rt.harnessProject(project)
		if err != nil {
			return err
		}
		if !validUUID(session) || !agentNameRE.MatchString(agent) {
			return usagef("--session and --agent are required")
		}
		me, err := rt.caller()
		if err != nil {
			return err
		}
		if me.Principal.Name != agent {
			return usagef("--agent must name the authenticated principal")
		}
		lease, err := harnessSecret(leaseFile, "worker-lease-file")
		if err != nil {
			return err
		}
		if len(lease) < 32 {
			return usagef("worker lease must contain at least 32 characters")
		}
		body := map[string]any{}
		switch kind {
		case "heartbeat":
			if phase == "" {
				phase = "working"
			}
			body = map[string]any{"phase": phase, "activity": activity, "activity_sequence": sequence}
		case "complete-delivery":
			if !validUUID(deliveryID) || cursor < 1 {
				return usagef("delivery id and positive cursor required")
			}
			if level == "" {
				level = "simple"
			}
			body = map[string]any{"delivery_id": deliveryID, "cursor": cursor, "effective_level": level}
		case "mark-stopped":
			if reason == "" {
				reason = "stopped"
			}
			body = map[string]any{"reason": reason}
		}
		path := harnessPath(id, session) + "/" + kind
		if kind == "mark-stopped" {
			path = harnessPath(id, session) + "/stop"
		}
		var out any
		err = rt.harnessDo(http.MethodPost, path, lease, body, &out)
		if err != nil {
			return err
		}
		return rt.printHarness(out)
	}}
}
func (rt *runtime) harnessControl(kind string) *Command {
	var project, session, controlID, agent, leaseFile, outcome, reason string
	return &Command{Name: kind, Short: "Request or complete an owned control", Use: "harness " + kind + " --project KEY --session UUID", addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 'p', "project key")
		fs.string(&session, "session", 0, "public session UUID")
		if kind == "complete-control" {
			fs.string(&controlID, "control-id", 0, "claimed control UUID")
			fs.string(&agent, "agent", 0, "attributed agent name")
			fs.string(&leaseFile, "worker-lease-file", 0, "private generation lease file")
			fs.string(&outcome, "outcome", 0, "applied or rejected")
			fs.string(&reason, "reason", 0, "closed completion reason")
		}
	}, run: func([]string) error {
		id, err := rt.harnessProject(project)
		if err != nil {
			return err
		}
		if !validUUID(session) {
			return usagef("--session UUID is required")
		}
		path := harnessPath(id, session) + "/controls/" + kind
		lease := ""
		body := map[string]any{}
		if kind == "complete-control" {
			if !validUUID(controlID) || !agentNameRE.MatchString(agent) {
				return usagef("--control-id and --agent are required")
			}
			me, e := rt.caller()
			if e != nil {
				return e
			}
			if me.Principal.Name != agent {
				return usagef("--agent must name the authenticated principal")
			}
			lease, err = harnessSecret(leaseFile, "worker-lease-file")
			if err != nil {
				return err
			}
			if len(lease) < 32 {
				return usagef("worker lease must contain at least 32 characters")
			}
			if outcome == "" {
				outcome = "applied"
			}
			if reason == "" {
				reason = outcome
			}
			body = map[string]any{"outcome": outcome, "reason": reason}
			path = harnessPath(id, session) + "/controls/" + controlID + "/complete"
		}
		var out any
		err = rt.harnessDo(http.MethodPost, path, lease, body, &out)
		if err != nil {
			return err
		}
		return rt.printHarness(out)
	}}
}

// harnessSessionFull is called by the session-start branch in compat_cmds.go.
// The bundle is a tenant API snapshot, cached in a private per-instance file.
func (rt *runtime) harnessSessionFull(project, agent, format, sid string) error {
	if !validUUID(sid) || !agentNameRE.MatchString(agent) {
		return usagef("invalid agent or session id")
	}
	if format != "env" && format != "json" && format != "files" {
		return usagef("invalid --format %q", format)
	}
	projectNode, err := rt.projectNode(project)
	if err != nil {
		return err
	}
	nodes, err := rt.walkNodes(nil, nil)
	if err != nil {
		return err
	}
	kinds, err := rt.loadKinds()
	if err != nil {
		return err
	}
	byID := make(map[string]apiNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	belongs := func(n apiNode) bool {
		for depth := 0; depth < 32; depth++ {
			if n.ID == projectNode.ID {
				return true
			}
			if n.ParentID == nil {
				return false
			}
			parent, ok := byID[*n.ParentID]
			if !ok {
				return false
			}
			n = parent
		}
		return false
	}
	relevant := []map[string]any{}
	byKind := map[string][]map[string]any{"memory": {}, "runbook": {}, "guideline": {}, "work_order": {}, "ticket": {}, "task": {}, "epic": {}}
	for _, n := range nodes {
		slug := kinds.slug(n.KindID)
		if belongs(n) {
			entry := map[string]any{"id": n.ID, "key": n.Key, "kind": slug, "title": n.Title, "body": n.Body, "fields": fieldMap(n.Fields)}
			relevant = append(relevant, entry)
			if _, ok := byKind[slug]; ok {
				byKind[slug] = append(byKind[slug], entry)
			}
		}
	}
	bundle := map[string]any{"schema": "aeon.session.bundle.v1", "project": map[string]any{"id": projectNode.ID, "key": projectNode.Key}, "agent_name": agent, "session_id": sid, "nodes": relevant, "memory": byKind["memory"], "runbooks": byKind["runbook"], "guidelines": byKind["guideline"], "tickets": byKind["ticket"], "tasks": byKind["task"], "epics": byKind["epic"], "work_orders": byKind["work_order"]}
	content, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(content)
	rev := hex.EncodeToString(sum[:])
	bundle["rev"] = rev
	if format == "json" {
		return rt.printJSON(bundle)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	config, err := rt.configFile()
	if err != nil {
		return err
	}
	dir := filepath.Join(filepath.Dir(config), "bundles", projectNode.ID, sid)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "manifest.json")
	if err = writePrivate(path, raw); err != nil {
		return err
	}
	if format == "files" {
		fmt.Fprintf(rt.stdout, "wrote bundle to %s (rev=%s)\n", dir, rev)
		return nil
	}
	safePath := "'" + strings.ReplaceAll(dir, "'", "'\\''") + "'"
	fmt.Fprintf(rt.stdout, "export PAIMOS_AGENT_NAME=%s\nexport PAIMOS_SESSION_ID=%s\nexport PAIMOS_KNOWLEDGE_DIR=%s\n", agent, sid, safePath)
	return nil
}
