// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"encoding/json"
	"testing"
)

func TestCodexSteerOnlyDowngradesDocumentedRejections(t *testing.T) {
	for _, tc := range []struct {
		name, raw, reason string
	}{
		{"idle", `{"code":-32600,"message":"no active turn to steer"}`, "idle"},
		{"review", `{"code":-32600,"message":"cannot steer a review turn"}`, "not_steerable"},
		{"race", "{\"code\":-32600,\"message\":\"expected active turn id `old` but found `new`\"}", "not_steerable"},
		{"unknown", `{"code":-32601,"message":"Method not found"}`, ""},
		{"ownership", `{"code":-32600,"message":"direct input is not allowed"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var remote codexRPCError
			if err := json.Unmarshal([]byte(tc.raw), &remote); err != nil {
				t.Fatal(err)
			}
			reason, ok := classifyCodexSteerRejection(&remote)
			if reason != tc.reason || ok != (tc.reason != "") {
				t.Fatalf("downgrade reason=%q accepted=%t", reason, ok)
			}
		})
	}
}
