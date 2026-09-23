// SPDX-License-Identifier: AGPL-3.0-only

package agentaccounts

import (
	"math"
	"testing"
	"time"
)

func TestPaceFormulas(t *testing.T) {
	if paceFraction("steady", 0.25, 0.1) != 0.35 {
		t.Fatal("steady")
	}
	if math.Abs(paceFraction("frontload", 0.5, 0.1)-0.85) > 1e-12 {
		t.Fatal("frontload")
	}
	if paceFraction("frontload", 0, 0.1) != 0.1 || paceFraction("unrestricted", 0, 0.1) != 1 {
		t.Fatal("edges")
	}
	if paceFraction("other", 1, 0.1) != 0 {
		t.Fatal("unknown pace must fail closed")
	}
	if allowedUnits(100, 0.1) != 10 || allowedUnits(100, 1) != 100 || allowedUnits(100, 0) != 0 {
		t.Fatal("allowed units")
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	window := Window{
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
		Allowance: 100, PaceModel: "frontload", BurstRatio: 0.1,
	}
	if _, ok := fits(window, now, 85); !ok {
		t.Fatal("85 at the midpoint should fit")
	}
	if _, ok := fits(window, now, 86); ok {
		t.Fatal("86 exceeds the frontload cap")
	}
	steady := window
	steady.StartsAt = now
	steady.EndsAt = now.Add(24 * time.Hour)
	steady.PaceModel = "steady"
	if _, ok := fits(steady, now, 10); !ok {
		t.Fatal("burst of 10 should fit at the open")
	}
	if _, ok := fits(steady, now, 11); ok {
		t.Fatal("burst of 11 should not fit")
	}
}
