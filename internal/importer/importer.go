// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Report describes one snapshot. Unmapped fields remain in fields.classic on
// imported nodes, or in classic.* event payloads for auxiliary resources.
type Report struct {
	Counts         map[string]int `json:"counts"`
	UnmappedFields []string       `json:"unmapped_fields"`
	Created        int            `json:"created"`
	Updated        int            `json:"updated"`
}

type Writer interface {
	Write(context.Context, Snapshot, string) (Report, error)
}

type Importer struct {
	Source Source
	Writer Writer
}

func (i Importer) Run(ctx context.Context, tenant, project string, dryRun bool) (Report, error) {
	if i.Source == nil {
		return Report{}, fmt.Errorf("source is required")
	}
	snap, err := i.Source.Read(ctx, project)
	if err != nil {
		return Report{}, err
	}
	report := Analyze(snap)
	if dryRun {
		return report, nil
	}
	if i.Writer == nil {
		return report, fmt.Errorf("writer is required")
	}
	return i.Writer.Write(ctx, snap, tenant)
}

var nativeProject = map[string]bool{"id": true, "key": true, "name": true, "description": true, "status": true, "created_at": true, "updated_at": true}
var nativeIssue = map[string]bool{"id": true, "issue_key": true, "title": true, "description": true, "body": true, "type": true, "status": true, "parent_id": true, "created_at": true, "updated_at": true}
var nativeUser = map[string]bool{"id": true, "username": true, "email": true, "role": true, "created_at": true}

func Analyze(s Snapshot) Report {
	r := Report{Counts: map[string]int{"users": len(s.Users), "projects": len(s.Projects)}}
	unmapped := map[string]bool{}
	collect := func(prefix string, record Record, native map[string]bool) {
		for k := range record {
			if !native[k] {
				unmapped[prefix+"."+k] = true
			}
		}
	}
	for _, u := range s.Users {
		collect("users", u, nativeUser)
	}
	for _, p := range s.Projects {
		collect("projects", p.Record, nativeProject)
		for _, issue := range p.Issues {
			typ := stringField(issue, "type")
			if typ == "" {
				typ = "unknown"
			}
			r.Counts[typ]++
			collect("issues", issue, nativeIssue)
		}
	}
	for _, issue := range s.Orphans {
		typ := stringField(issue, "type")
		if typ == "" {
			typ = "unknown"
		}
		r.Counts[typ]++
		collect("issues", issue, nativeIssue)
	}
	relations := map[string]bool{}
	for _, d := range s.Details {
		r.Counts["comments"] += len(d.Comments)
		r.Counts["history"] += len(d.History)
		r.Counts["attachments_metadata"] += len(d.Attachments)
		for _, a := range d.Attachments {
			if stringField(a, "object_key") != "" {
				unmapped["attachments.file_bytes"] = true
			}
		}
		for _, rel := range d.Relations {
			sourceID, _ := intField(rel, "source_id")
			targetID, _ := intField(rel, "target_id")
			ref := fmt.Sprintf("%s:%d:%d", stringField(rel, "type"), sourceID, targetID)
			relations[ref] = true
			switch stringField(rel, "type") {
			case "parent", "depends_on", "relates", "duplicates", "cites":
			default:
				unmapped["relations."+stringField(rel, "type")] = true
			}
		}
	}
	r.Counts["relations"] = len(relations)
	for k := range unmapped {
		r.UnmappedFields = append(r.UnmappedFields, k)
	}
	sort.Strings(r.UnmappedFields)
	return r
}

func jsonValue(v any) ([]byte, error) { return json.Marshal(v) }
func canonicalType(t string) string   { return strings.ReplaceAll(t, "-", "_") }
