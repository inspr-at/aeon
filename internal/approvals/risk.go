// SPDX-License-Identifier: AGPL-3.0-only

package approvals

import "strings"

// Risk is a presentation hint, never an authority or an alternative to a grant.
// Tenant-wide requests and dangerous scope segments take priority over read.
func Risk(scope, resourceKind string) string {
	if resourceKind == "tenant" {
		return "high"
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
