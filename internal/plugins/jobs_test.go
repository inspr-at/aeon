// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"errors"
	"testing"
	"time"
)

func TestLeaseIsTenantScoped(t *testing.T) {
	leases := &leaseTable{held: map[string]lease{}}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	release, err := leases.acquire(leaseKey("tenant-a", "lab", "sweep"), "holder", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leases.acquire(leaseKey("tenant-a", "lab", "sweep"), "other", now, time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("same tenant = %v", err)
	}
	if _, err := leases.acquire(leaseKey("tenant-b", "lab", "sweep"), "other", now, time.Minute); err != nil {
		t.Fatalf("other tenant = %v", err)
	}
	if _, err := leases.acquire(leaseKey("tenant-a", "lab", "sweep"), "other", now.Add(time.Minute), time.Minute); err != nil {
		t.Fatalf("expired = %v", err)
	}
	release()
	if _, err := leases.acquire(leaseKey("tenant-a", "lab", "sweep"), "third", now.Add(time.Minute), time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("release removed a replacement lease: %v", err)
	}
}
