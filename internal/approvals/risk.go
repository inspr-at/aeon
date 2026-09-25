// SPDX-License-Identifier: AGPL-3.0-only

package approvals

import (
	"strings"

	"github.com/inspr-at/aeon/internal/authz"
)

// Risk classifies requests for the person decision gate; it grants no authority.
// Tenant-wide requests and dangerous scope segments take priority over read.
func Risk(scope, resourceKind string) string {
	if resourceKind == "tenant" {
		return "high"
	}
	if permission := approvalPermission(scope); permission != "" {
		entry, _ := authz.Lookup(permission)
		return entry.Risk
	}
	parts := strings.Split(scope, ".")
	for _, part := range parts {
		switch part {
		case "control", "deploy", "delete":
			return "high"
		}
	}
	if len(parts) > 1 && parts[1] == "read" {
		return "low"
	}
	return "medium"
}
