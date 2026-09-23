// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/inspr-at/aeon/internal/inbox"
)

var messagingAddressRE = regexp.MustCompile(`^(paimos|codex|claude|pi|cursor|grok|grok_bot):[a-z][a-z0-9_-]{0,63}$`)

func (rt *runtime) cmdMessaging() *Command {
	return &Command{Name: "message", Short: "Messaging targets and redacted deliveries", Use: "message <target|deliveries>", subs: []*Command{
		{Name: "target", Short: "Versioned delivery targets", Use: "message target <set|list>", subs: []*Command{rt.cmdMessagingTargetSet(), rt.cmdMessagingTargetList()}}, rt.cmdMessagingDeliveries(),
	}}
}
func (rt *runtime) cmdMessagingTargetSet() *Command {
	var project, address, adapter, kind, principal, webhook, keyFile, refFile, level, role string
	return &Command{Name: "set", Short: "Register an encrypted target", Use: "message target set --project KEY --address harness:agent --adapter NAME --kind KIND --target-ref-file FILE", maxArgs: 0, addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 0, "project key")
		fs.string(&address, "address", 0, "harness:agent address")
		fs.string(&adapter, "adapter", 0, "codex, agentd_codex, agentd_claude, agentd_pi, agentd_cursor, grok_bot_routine, claude_resume or claude_channel")
		fs.string(&kind, "kind", 0, "codex_thread, agentd_session, https_webhook, claude_session, pull or webhook")
		fs.string(&principal, "principal", 0, "principal UUID (optional address identity check)")
		fs.string(&webhook, "webhook-url", 0, "R2 webhook URL")
		fs.string(&keyFile, "target-key-file", 0, "owner-only routine sender-key file")
		fs.string(&refFile, "target-ref-file", 0, "private target reference file, or - for stdin")
		fs.string(&level, "maximum-level", 0, "simple (default) or steer")
		fs.string(&role, "role", 0, "primary (default) or simple_fallback")
	}, run: func(args []string) error {
		if address == "" && adapter == "" && !classicTargetKind(kind) && keyFile == "" && refFile == "" {
			if level != "" || role != "" {
				return usagef("--maximum-level and --role require a classic target")
			}
			return rt.setR2MessagingTarget(project, principal, kind, webhook)
		}
		if project == "" || !messagingAddressRE.MatchString(address) || adapter == "" || kind == "" || refFile == "" {
			return usagef("--project, --address, --adapter, --kind and --target-ref-file are required")
		}
		if webhook != "" {
			return usagef("use --target-ref-file for classic targets")
		}
		if keyFile != "" && adapter != "grok_bot_routine" {
			return usagef("--target-key-file is only valid for grok_bot_routine")
		}
		if adapter == "grok_bot_routine" && keyFile == "" {
			return usagef("--target-key-file is required for grok_bot_routine")
		}
		if keyFile == "-" {
			return usagef("--target-key-file must be an owner-only file")
		}
		if principal != "" && !validUUID(principal) {
			return usagef("--principal must be a UUID")
		}
		if level == "" {
			level = "simple"
		}
		if role == "" {
			role = "primary"
		}
		ref, err := rt.readMessagingPrivateFile(refFile, false)
		if err != nil {
			return err
		}
		var secret string
		if keyFile != "" {
			secret, err = rt.readMessagingPrivateFile(keyFile, true)
			if err != nil {
				return err
			}
		}
		p, err := rt.projectNode(project)
		if err != nil {
			return err
		}
		req := map[string]any{"address": address, "adapter": adapter, "target_kind": kind, "target_ref": ref, "target_secret": secret, "maximum_level": level, "role": role}
		if principal != "" {
			req["principal_id"] = principal
		}
		var created inbox.MessageTarget
		if err := rt.privateMessagingPost("/api/projects/"+url.PathEscape(p.ID)+"/message-targets", req, &created); err != nil {
			return err
		}
		if rt.jsonOut {
			return rt.printJSON(created)
		}
		_, err = fmt.Fprintf(rt.stdout, "✓ enabled target %s for %s (%s, version %d)\n", created.ID, created.Address, created.Adapter, created.Version)
		return err
	}}
}

func (rt *runtime) readMessagingPrivateFile(path string, key bool) (string, error) {
	var reader io.Reader
	if path == "-" && !key {
		reader = rt.stdin
	} else {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", usagef("target input must be a readable regular file")
		}
		if key && info.Mode().Perm()&0077 != 0 {
			return "", usagef("target key file must be owner-only (0600)")
		}
		f, err := os.Open(path)
		if err != nil {
			return "", usagef("cannot read target input file")
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return "", usagef("target input changed while opening")
		}
		reader = f
	}
	raw, err := io.ReadAll(io.LimitReader(reader, 8193))
	if err != nil || len(raw) > 8192 {
		return "", usagef("cannot read bounded target input")
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", usagef("target input is empty")
	}
	return value, nil
}

// Never follow redirects with write-only target credentials, and never print
// a server error body: a reverse proxy may echo submitted private material.
func (rt *runtime) privateMessagingPost(path string, body, dest any) error {
	c, err := rt.api()
	if err != nil {
		return err
	}
	c.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if err := c.Do(context.Background(), http.MethodPost, path, body, dest); err != nil {
		return rt.fail(errors.New("target registration failed; private server details suppressed"), "")
	}
	return nil
}
func (rt *runtime) setR2MessagingTarget(project, principal, kind, webhook string) error {
	if kind != "pull" && kind != "webhook" {
		return usagef("--kind must be pull or webhook")
	}
	if (kind == "webhook") != (webhook != "") {
		return usagef("--webhook-url is required only for webhook targets")
	}
	if project != "" {
		if _, err := rt.projectNode(project); err != nil {
			return err
		}
	}
	if principal == "" {
		me, err := rt.caller()
		if err != nil {
			return err
		}
		principal = me.Principal.ID
	}
	if !validUUID(principal) {
		return usagef("--principal must be a UUID")
	}
	req := map[string]any{"principal_id": principal, "kind": kind}
	if kind == "webhook" {
		req["webhook_url"] = webhook
	}
	var created inboxTarget
	if err := rt.privateMessagingPost("/api/inbox/targets", req, &created); err != nil {
		return err
	}
	if rt.jsonOut {
		return rt.printJSON(targetView(created))
	}
	_, err := fmt.Fprintf(rt.stdout, "✓ enabled target %s for %s (%s)\n", created.ID, created.PrincipalID, created.Kind)
	return err
}
func (rt *runtime) cmdMessagingTargetList() *Command {
	var project, address string
	return &Command{Name: "list", Short: "List redacted targets", Use: "message target list [--project KEY] [--address harness:agent]", maxArgs: 0, addFlags: func(fs *flagSet) {
		fs.string(&project, "project", 0, "project key for classic targets")
		fs.string(&address, "address", 0, "optional harness address")
	}, run: func(args []string) error {
		if project == "" {
			if address != "" {
				return usagef("--project is required with --address")
			}
			return rt.cmdMessageTargetList().run(nil)
		}
		p, err := rt.projectNode(project)
		if err != nil {
			return err
		}
		var items []inbox.MessageTarget
		if err := rt.do(http.MethodGet, "/api/projects/"+url.PathEscape(p.ID)+"/message-targets?"+url.Values{"address": {address}}.Encode(), nil, &items); err != nil {
			return err
		}
		if rt.jsonOut {
			return rt.printJSON(items)
		}
		if len(items) == 0 {
			fmt.Fprintln(rt.stdout, "(no targets)")
		}
		for _, v := range items {
			state := "disabled"
			if v.Enabled {
				state = "enabled"
			}
			fmt.Fprintf(rt.stdout, "%s  %s  %s  %s  v%d  %s  has_secret=%t\n", v.ID, v.Address, v.Adapter, v.Role, v.Version, state, v.HasSecret)
		}
		return nil
	}}
}
func (rt *runtime) cmdMessagingDeliveries() *Command {
	var project string
	return &Command{Name: "deliveries", Short: "Inspect redacted delivery state", Use: "message deliveries --project KEY", maxArgs: 0, addFlags: func(fs *flagSet) { fs.string(&project, "project", 0, "project key (required)") }, run: func(args []string) error {
		if project == "" {
			return usagef("--project is required")
		}
		p, err := rt.projectNode(project)
		if err != nil {
			return err
		}
		var items []inbox.MessageDelivery
		if err := rt.do(http.MethodGet, "/api/projects/"+url.PathEscape(p.ID)+"/message-deliveries", nil, &items); err != nil {
			return err
		}
		if rt.jsonOut {
			return rt.printJSON(items)
		}
		if len(items) == 0 {
			fmt.Fprintln(rt.stdout, "(no deliveries)")
		}
		for _, v := range items {
			fmt.Fprintf(rt.stdout, "%s  message=%s  %s  %s  attempts=%d\n", v.ID, v.MessageID, v.State, v.Reason, v.Attempts)
		}
		return nil
	}}
}
