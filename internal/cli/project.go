// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// project list retains the classic inventory shape for agent routing. Project
// identity is the Aeon node UUID; project_key is the imported classic key.
type projectView struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (rt *runtime) cmdProject() *Command {
	return &Command{Name: "project", Short: "Projects", Use: "project <list>", subs: []*Command{rt.cmdProjectList()}}
}

func (rt *runtime) cmdProjectList() *Command {
	var status string
	var archived, all bool
	return &Command{Name: "list", Short: "List projects on the current instance", Use: "project list [--status active|frozen|archived|deleted|all]", addFlags: func(fs *flagSet) {
		fs.bool(&archived, "archived", 0, "list archived projects")
		fs.bool(&all, "all", 0, "list every project status")
		fs.string(&status, "status", 0, "active, frozen, archived, deleted, or all")
	}, run: func([]string) error {
		if (archived && all) || (status != "" && (archived || all)) {
			return usagef("--archived, --all, and --status are mutually exclusive")
		}
		status = strings.ToLower(strings.TrimSpace(status))
		if archived {
			status = "archived"
		}
		if all {
			status = "all"
		}
		switch status {
		case "", "active", "frozen", "archived", "deleted", "all":
		default:
			return usagef("--status must be active, frozen, archived, deleted, or all")
		}
		kind, err := rt.kind("project")
		if err != nil {
			return err
		}
		nodes, err := rt.walkNodes(url.Values{"kind_id": {kind.ID}}, nil)
		if err != nil {
			return err
		}
		views := make([]projectView, 0, len(nodes))
		for _, n := range nodes {
			state := n.State
			if state == "" {
				state = "active"
			}
			if status == "" || status == "active" {
				if state != "active" {
					continue
				}
			} else if status != "all" && state != status {
				continue
			}
			views = append(views, projectView{ID: n.ID, Key: projectDisplayKey(n, n.Key), Name: n.Title, Description: n.Body, Status: state})
		}
		sort.Slice(views, func(i, j int) bool { return views[i].Key < views[j].Key })
		if rt.jsonOut {
			return rt.printJSON(views)
		}
		if len(views) == 0 {
			_, err = fmt.Fprintln(rt.stdout, "(no projects)")
			return err
		}
		if _, err = fmt.Fprintln(rt.stdout, "KEY           STATUS     NAME"); err != nil {
			return err
		}
		for _, view := range views {
			if _, err = fmt.Fprintf(rt.stdout, "%-13s %-10s %s\n", view.Key, view.Status, view.Name); err != nil {
				return err
			}
		}
		return nil
	}}
}
