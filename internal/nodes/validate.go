// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	slugRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
	keyRe    = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-[1-9][0-9]*$`)
	uuidRe   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func parseUUID(s string) (string, bool) {
	if !uuidRe.MatchString(s) {
		return "", false
	}
	return strings.ToLower(s), true
}

func validSlug(s string) bool { return slugRe.MatchString(s) }

func validPrefix(s string) bool { return prefixRe.MatchString(s) }

func validKey(s string) bool { return len(s) <= 30 && keyRe.MatchString(s) }

func nonBlank(s string) bool { return strings.TrimSpace(s) != "" }

func parseStringList(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, badRequest("allowed_child_kinds must be an array of slugs or null")
	}
	if items == nil {
		items = []string{}
	}
	seen := map[string]bool{}
	for _, slug := range items {
		if !validSlug(slug) {
			return nil, badRequest("allowed_child_kinds contains an invalid slug")
		}
		if seen[slug] {
			return nil, badRequest("allowed_child_kinds contains a duplicate slug")
		}
		seen[slug] = true
	}
	return items, nil
}

func childAllowed(allowed []string, childSlug string) bool {
	if allowed == nil {
		return true
	}
	for _, slug := range allowed {
		if slug == childSlug {
			return true
		}
	}
	return false
}
