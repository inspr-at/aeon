// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"fmt"
	"net/http"
	"strings"
)

func (rt *runtime) cmdRelation() *Command {
	return &Command{Name: "relation", Short: "Operate on issue relations", Use: "relation add <source-ref> <type> <target-ref>", subs: []*Command{
		{Name: "add", Short: "Add a relation between two nodes", Use: "relation add <source-ref> <type> <target-ref>", minArgs: 3, maxArgs: 3,
			run: func(args []string) error {
				for _, ref := range []string{args[0], args[2]} {
					if strings.HasPrefix(ref, "id:") {
						return usagef("id:<n> is a classic numeric id; pass a node key or UUID")
					}
				}
				typ := strings.ToLower(strings.TrimSpace(args[1]))
				// Import-compatible legacy types collapse to Aeon's closed set.
				typ, reverse := aeonRelationType(typ)
				switch typ {
				case "blocks", "relates", "implements", "cites", "duplicates", "customer_of", "contact_for":
				default:
					return usagef("unknown relation type %q", args[1])
				}
				src, err := rt.nodeRef(args[0])
				if err != nil {
					return err
				}
				tgt, err := rt.nodeRef(args[2])
				if err != nil {
					return err
				}
				if reverse {
					src, tgt = tgt, src
				}
				var created map[string]any
				if err := rt.do(http.MethodPost, "/api/relations", map[string]any{"source_node_id": src.ID, "target_node_id": tgt.ID, "type": typ}, &created); err != nil {
					return err
				}
				if rt.jsonOut {
					return rt.printJSON(created)
				}
				_, err = fmt.Fprintf(rt.stdout, "✓ %s %s %s\n", args[0], strings.ToLower(strings.TrimSpace(args[1])), args[2])
				return err
			}},
	}}
}

func aeonRelationType(typ string) (string, bool) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "related", "follows_from", "impacts", "applies_to_memory", "groups":
		return "relates", false
	case "depends_on":
		return "blocks", true
	default:
		return typ, false
	}
}

func (rt *runtime) nodeRef(ref string) (apiNode, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return apiNode{}, usagef("node reference is required")
	}
	if validUUID(ref) {
		var n apiNode
		if err := rt.do(http.MethodGet, "/api/nodes/"+ref, nil, &n); err != nil {
			return apiNode{}, err
		}
		return n, nil
	}
	return rt.nodeByKey(ref)
}
