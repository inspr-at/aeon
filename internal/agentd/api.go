// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/inspr-at/aeon/internal/client"
)

// Remote uses a scoped agent key through AEON's public HTTP contract.
// NewRemote callers should pin the base URL to HTTPS or a trusted loopback.
type Remote struct {
	Client               *client.Client
	mu                   sync.RWMutex
	daemonID, generation string
}

func NewRemote(baseURL, token string) *Remote { return &Remote{Client: client.New(baseURL, token)} }

func (r *Remote) Identity(ctx context.Context) (string, string, error) {
	me, err := r.Client.Me(ctx)
	if err != nil {
		return "", "", err
	}
	if me.Principal.Kind != "agent" || me.Principal.ID == "" || me.Tenant.ID == "" || me.Principal.TenantID != me.Tenant.ID {
		return "", "", errors.New("agent key identity is invalid")
	}
	return me.Tenant.ID, me.Principal.ID, nil
}

func (r *Remote) Queued(ctx context.Context) ([]Run, error) {
	var runs []Run
	err := r.Client.Do(ctx, "GET", "/api/runs/queued?limit=100", nil, &runs)
	return runs, err
}

func (r *Remote) GetRun(ctx context.Context, id string) (Run, error) {
	var run Run
	err := r.Client.Do(ctx, "GET", "/api/runs/"+url.PathEscape(id), nil, &run)
	return run, err
}

func (r *Remote) Profiles(ctx context.Context) ([]Profile, error) {
	var profiles []Profile
	err := r.Client.Do(ctx, "GET", "/api/models", nil, &profiles)
	return profiles, err
}

func (r *Remote) Node(ctx context.Context, id string) (Node, error) {
	var node Node
	err := r.Client.Do(ctx, "GET", "/api/nodes/"+url.PathEscape(id), nil, &node)
	return node, err
}

func (r *Remote) WorkOrder(ctx context.Context, id string) (WorkOrder, error) {
	var order WorkOrder
	err := r.Client.Do(ctx, "GET", "/api/work-orders/"+url.PathEscape(id), nil, &order)
	return order, err
}

func (r *Remote) Route(ctx context.Context, runID, daemonID string, accountIDs []string, estimates map[string]int64) (Route, error) {
	var route Route
	err := r.Client.Do(ctx, "POST", "/api/agent-accounts/route", map[string]any{
		"run_id": runID, "daemon_id": daemonID, "account_ids": accountIDs, "estimated_units": estimates,
	}, &route)
	return route, err
}

func (r *Remote) Claim(ctx context.Context, runID, daemonID, generation string, reservations []string) error {
	err := r.Client.Do(ctx, "POST", "/api/runs/"+url.PathEscape(runID)+"/claim", map[string]any{
		"daemon_id": daemonID, "daemon_generation": generation, "reservation_ids": reservations,
	}, nil)
	if err == nil {
		r.mu.Lock()
		r.daemonID, r.generation = daemonID, generation
		r.mu.Unlock()
	}
	return err
}

func (r *Remote) Report(ctx context.Context, runID string, t Telemetry) error {
	r.mu.RLock()
	daemon, generation := r.daemonID, r.generation
	r.mu.RUnlock()
	if daemon == "" || generation == "" {
		return errors.New("run has no daemon claim")
	}
	return r.Client.DoWithHeaders(ctx, "POST", "/api/runs/"+url.PathEscape(runID)+"/telemetry", t, nil,
		map[string]string{"X-Aeon-Daemon-ID": daemon, "X-Aeon-Daemon-Generation": generation})
}

func (r *Remote) Inbox(ctx context.Context, after int64) (InboxPage, error) {
	var page InboxPage
	err := r.Client.Do(ctx, "GET", fmt.Sprintf("/api/inbox/messages?after=%d&wait_ms=0&limit=100", after), nil, &page)
	return page, err
}

func (r *Remote) Ack(ctx context.Context, id string) error {
	return r.Client.Do(ctx, "POST", "/api/inbox/messages/"+url.PathEscape(id)+"/ack", nil, nil)
}

func (r *Remote) AddEvidence(ctx context.Context, workOrderID, runID, answer string) error {
	if answer == "" || len(answer) > 64<<10 {
		return errors.New("run evidence is empty or exceeds bound")
	}
	return r.Client.Do(ctx, "POST", "/api/work-orders/"+url.PathEscape(workOrderID)+"/evidence",
		map[string]any{"kind": "text", "reference": answer, "run_id": runID}, nil)
}

func (r *Remote) Probe(ctx context.Context, accountID, daemonID, generation string, available bool) error {
	return r.Client.Do(ctx, "POST", "/api/agent-accounts/"+url.PathEscape(accountID)+"/probe",
		map[string]any{"daemon_id": daemonID, "daemon_generation": generation, "available": available}, nil)
}

// ValidateBaseURL rejects credential-bearing and remote cleartext endpoints.
func ValidateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(raw) != raw {
		return errors.New("invalid AEON URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
		return nil
	}
	return errors.New("AEON URL must use HTTPS outside loopback")
}
