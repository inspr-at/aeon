// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
)

type lease struct {
	holder string
	until  time.Time
}

type leaseTable struct {
	mu   sync.Mutex
	held map[string]lease
}

func (t *leaseTable) acquire(key, holder string, now time.Time, ttl time.Duration) (func(), error) {
	if ttl <= 0 || holder == "" || key == "" {
		return nil, ErrLeaseHeld
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur, ok := t.held[key]; ok && now.Before(cur.until) {
		return nil, ErrLeaseHeld
	}
	t.held[key] = lease{holder: holder, until: now.Add(ttl)}
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		cur, ok := t.held[key]
		if ok && cur.holder == holder && cur.until.Equal(now.Add(ttl)) {
			delete(t.held, key)
		}
	}, nil
}

func leaseKey(tenantID, pluginID, jobID string) string {
	return tenantID + "\x00" + pluginID + "\x00" + jobID
}

// RunJob runs one declared job under a tenant-scoped lease and appends
// plugin.job_ran. The job does not see the pool. A held lease writes nothing.
func (m *Module) RunJob(ctx context.Context, p tenant.Principal, pluginID, jobID string) error {
	plug, ok := m.reg.plugin(pluginID)
	if !ok || plug.Jobs == nil {
		return notFound("plugin has no background jobs")
	}
	perm, ok := capabilityPerm(plug.Manifest.BackgroundJobs, jobID)
	if !ok {
		return notFound("background job not found")
	}
	if _, _, err := m.bind(ctx, p, pluginID, needPerms(fence.PermJobsRun, perm)); err != nil {
		return err
	}
	release, err := m.leases.acquire(leaseKey(p.TenantID, pluginID, jobID), p.ID, m.now(), m.leaseTTL)
	if err != nil {
		return err
	}
	defer release()
	call, plug, err := m.bind(ctx, p, pluginID, needPerms(fence.PermJobsRun, perm))
	if err != nil {
		return err
	}
	result, runErr := plug.Jobs.Run(ctx, call, jobID)
	outcome := "failed"
	if runErr == nil && result.Outcome == "succeeded" {
		outcome = "succeeded"
	}
	return m.inTenant(tenant.WithPrincipal(ctx, p), p.TenantID, func(tx pgx.Tx) error {
		_, err := m.appendEvent(ctx, tx, p, events.Change{
			Type: eventJobRan,
			After: map[string]string{
				"plugin_id":              pluginID,
				"job_id":                 jobID,
				"manifest_digest_sha256": plug.Manifest.DigestSHA256,
				"outcome":                outcome,
			},
		})
		return err
	})
}
