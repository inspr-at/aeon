// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type movedIssue struct {
	IssueID   string   `json:"issue_id"`
	OldKey    string   `json:"old_key"`
	NewKey    string   `json:"new_key"`
	ProjectID string   `json:"project_id"`
	Detached  []string `json:"detached"`
	Notes     []string `json:"notes"`
}

func (rt *runtime) moveIssues(refs []string, target string, dryRun bool) error {
	project, err := rt.projectNode(target)
	if err != nil {
		return err
	}
	nodes := make([]apiNode, 0, len(refs))
	for _, ref := range refs {
		if strings.HasPrefix(ref, "id:") {
			return usagef("id:<n> is a classic numeric id; pass the issue key")
		}
		node, err := rt.nodeByKey(ref)
		if err != nil {
			return err
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 1 {
		path := "/api/nodes/" + url.PathEscape(nodes[0].ID) + "/project-move"
		body := map[string]any{"project_id": project.ID}
		if dryRun {
			return rt.printJSON(map[string]any{"dry_run": true, "method": http.MethodPost, "path": path, "body": body})
		}
		var moved movedIssue
		if err := rt.do(http.MethodPost, path, body, &moved); err != nil {
			return err
		}
		if rt.jsonOut {
			return rt.printJSON(moved)
		}
		printMovedIssue(rt, moved)
		return nil
	}
	if dryRun {
		ids := make([]string, 0, len(nodes))
		for _, node := range nodes {
			ids = append(ids, node.ID)
		}
		return rt.printJSON(map[string]any{"dry_run": true, "method": http.MethodPost, "path": "/api/nodes/project-move", "body": map[string]any{"issue_ids": ids, "project_id": project.ID}})
	}
	type item struct {
		IssueID string      `json:"issue_id"`
		OK      bool        `json:"ok"`
		Error   string      `json:"error,omitempty"`
		Result  *movedIssue `json:"result,omitempty"`
	}
	results := make([]item, 0, len(nodes))
	movedCount := 0
	for _, node := range nodes {
		var moved movedIssue
		err := rt.do(http.MethodPost, "/api/nodes/"+url.PathEscape(node.ID)+"/project-move", map[string]any{"project_id": project.ID}, &moved)
		if err != nil {
			results = append(results, item{IssueID: node.ID, Error: err.Error()})
			continue
		}
		results = append(results, item{IssueID: node.ID, OK: true, Result: &moved})
		movedCount++
	}
	if rt.jsonOut {
		if err := rt.printJSON(map[string]any{"moved": movedCount, "failed": len(nodes) - movedCount, "results": results}); err != nil {
			return err
		}
		if movedCount != len(nodes) {
			return rt.fail(fmt.Errorf("%d issue moves failed", len(nodes)-movedCount), "")
		}
		return nil
	}
	for _, res := range results {
		if res.OK {
			printMovedIssue(rt, *res.Result)
		} else {
			fmt.Fprintf(rt.stdout, "✗ issue %s — %s\n", res.IssueID, res.Error)
		}
	}
	fmt.Fprintf(rt.stdout, "moved %d, failed %d\n", movedCount, len(nodes)-movedCount)
	if movedCount != len(nodes) {
		return rt.fail(fmt.Errorf("%d issue moves failed", len(nodes)-movedCount), "")
	}
	return nil
}

func printMovedIssue(rt *runtime, moved movedIssue) {
	fmt.Fprintf(rt.stdout, "✓ moved %s → %s\n", moved.OldKey, moved.NewKey)
	for _, name := range moved.Detached {
		fmt.Fprintf(rt.stdout, "  detached: %s\n", name)
	}
	for _, note := range moved.Notes {
		fmt.Fprintf(rt.stdout, "  note: %s\n", note)
	}
}
