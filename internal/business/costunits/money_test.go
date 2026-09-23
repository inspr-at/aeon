// SPDX-License-Identifier: AGPL-3.0-only

package costunits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
)

func TestAmountIsExactDecimal(t *testing.T) {
	for _, raw := range []string{"", "0.00001", "-1", "1e-5", "100000000000000", "NaN", "1/2", "10.10001", "+1", "inf"} {
		if _, err := parseAmount(json.Number(raw)); err == nil {
			t.Fatalf("%s accepted", raw)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"0", "0"},
		{"10.10", "10.1"},
		{"10.10000", "10.1"},
		{"10.5000", "10.5"},
		{"0.0001", "0.0001"},
		{"1e-4", "0.0001"},
		{"1.2e1", "12"},
		{"99999999999999.9999", "99999999999999.9999"},
	} {
		got, err := parseAmount(json.Number(tc.in))
		if err != nil || got.String() != tc.want {
			t.Fatalf("%s -> %s %v, want %s", tc.in, got, err, tc.want)
		}
		raw, err := json.Marshal(got)
		if err != nil || string(raw) != tc.want {
			t.Fatalf("json %s -> %s %v", tc.in, raw, err)
		}
	}
}

func TestAdjacentIntervalsDoNotOverlap(t *testing.T) {
	if overlaps("2026-01-01", "2026-06-01", "2026-06-01", "") {
		t.Fatal("adjacent intervals overlapped")
	}
	if !overlaps("2026-01-01", "", "2026-03-01", "2026-04-01") {
		t.Fatal("open interval did not cover March")
	}
	if overlaps("2026-06-01", "", "2025-01-01", "2025-06-01") {
		t.Fatal("a later open interval covered an earlier bounded one")
	}
}

func TestPluginRegistersBusinessCosts(t *testing.T) {
	plug, err := Plugin()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := plugins.Digest(plug)
	if err != nil || sum != plug.Manifest.DigestSHA256 || len(sum) != 64 {
		t.Fatalf("digest %s %v", sum, err)
	}
	reg := plugins.NewRegistry()
	if err := reg.Register(plug); err != nil {
		t.Fatal(err)
	}
	got, ok := reg.Lookup(PluginID)
	if !ok {
		t.Fatal("missing plugin")
	}
	raw, err := fieldSchema()
	if err != nil || !bytes.Equal(bytes.TrimSpace(raw), bytes.TrimSpace(got.Manifest.NodeKinds[0].FieldSchema)) {
		t.Fatalf("schema drift %s vs %s (%v)", raw, got.Manifest.NodeKinds[0].FieldSchema, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != true {
		t.Fatal("classic fields would be rejected")
	}
	if !slices.Equal(got.Manifest.Permissions, []string{fence.PermNodesContribute, fence.PermStepsApply, fence.PermViewsProvide}) {
		t.Fatalf("permissions %#v", got.Manifest.Permissions)
	}
	if got.StepPermissions[StepKey] != fence.PermStepsApply {
		t.Fatal("step permission")
	}
	if len(got.Manifest.AgentTools)+len(got.Manifest.Integrations)+len(got.Manifest.BackgroundJobs) != 0 {
		t.Fatal("undeclared capability")
	}
	if got.Manifest.NodeKinds[0].Slug != KindSlug || got.Manifest.Views[0].ID != ViewID {
		t.Fatal("kind or view")
	}
	if !slices.Equal(got.Manifest.WorkflowSteps[0].Gates, []string{fence.GateObservedState, fence.GatePersonDecision}) {
		t.Fatalf("gates %#v", got.Manifest.WorkflowSteps[0].Gates)
	}
	if _, err := plug.Kinds.NodeKinds(context.Background(), plugins.Call{}); !errors.Is(err, plugins.ErrDenied) {
		t.Fatalf("kind without grant: %v", err)
	}
	if _, err := plug.Steps.Evaluate(context.Background(), plugins.Call{}, plugins.StepRequest{Operation: StepKey}); !errors.Is(err, plugins.ErrDenied) {
		t.Fatalf("step without grant: %v", err)
	}
}
