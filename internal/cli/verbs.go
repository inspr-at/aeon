// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The issue, knowledge and search verbs match the paimos commands INSPR
// doctrine invokes. The Aeon HTTP contract does not include those resources
// yet, so a well-formed invocation exits 3 with "arrives in R1". model resolve
// and onboard are explicit stubs.

var knowledgeTypes = []string{"memory", "runbook", "guideline", "external-system", "related-project"}

func (rt *runtime) cmdIssue() *Command {
	return &Command{
		Name:  "issue",
		Short: "Issues",
		Use:   "issue <command>",
		subs: []*Command{
			rt.cmdIssueList(),
			rt.cmdIssueGet(),
			rt.cmdIssueCreate(),
			rt.cmdIssueUpdate(),
			rt.cmdIssueComment(),
			rt.cmdSearch("issue search"),
		},
	}
}

func (rt *runtime) cmdIssueList() *Command {
	var project, status, typ, priority, assignee string
	limit := 50
	offset := 0
	return &Command{
		Name:  "list",
		Short: "List issues",
		Use:   "issue list [flags]",
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 'p', "filter by project key")
			fs.string(&status, "status", 0, "filter by status")
			fs.string(&typ, "type", 0, "filter by type")
			fs.string(&priority, "priority", 0, "filter by priority")
			fs.string(&assignee, "assignee", 0, "filter by assignee id")
			fs.int(&limit, "limit", "page size")
			fs.int(&offset, "offset", "pagination offset")
		},
		run: func(args []string) error {
			if limit < 0 || offset < 0 {
				return usagef("--limit and --offset must be 0 or greater")
			}
			return arrives("issue list")
		},
	}
}

func (rt *runtime) cmdIssueGet() *Command {
	return &Command{
		Name:    "get",
		Short:   "Fetch one issue by key",
		Use:     "issue get <ref>",
		minArgs: 1,
		maxArgs: 1,
		run: func(args []string) error {
			if _, err := normalizeIssueRef(args[0]); err != nil {
				return err
			}
			return arrives("issue get")
		},
	}
}

func (rt *runtime) cmdIssueCreate() *Command {
	var project, title, typ, status, priority, parent, assignee string
	var description, descriptionFile, ac, acFile, notes, notesFile string
	var tags []string
	var dryRun bool
	return &Command{
		Name:  "create",
		Short: "Create an issue",
		Use:   "issue create --project KEY --title TITLE",
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 'p', "project key (required)")
			fs.string(&title, "title", 0, "title (required)")
			fs.string(&typ, "type", 0, "epic, ticket, task, …")
			fs.string(&status, "status", 0, "initial status")
			fs.string(&priority, "priority", 0, "low, medium, or high")
			fs.string(&parent, "parent", 0, "parent issue key or id:<n>")
			fs.string(&assignee, "assignee", 0, "assignee id")
			fs.string(&description, "description", 0, "inline description")
			fs.string(&descriptionFile, "description-file", 0, "description file, or - for stdin")
			fs.string(&ac, "ac", 0, "inline acceptance criteria")
			fs.string(&acFile, "ac-file", 0, "acceptance criteria file")
			fs.string(&notes, "notes", 0, "inline notes")
			fs.string(&notesFile, "notes-file", 0, "notes file")
			fs.strings(&tags, "tags", "tag name (repeatable)")
			fs.bool(&dryRun, "dry-run", 0, "accepted; the API is not available yet")
		},
		run: func(args []string) error {
			if strings.TrimSpace(project) == "" {
				return usagef("--project is required")
			}
			if strings.TrimSpace(title) == "" {
				return usagef("--title is required")
			}
			if err := exclusive(description, descriptionFile, "description"); err != nil {
				return err
			}
			if err := exclusive(ac, acFile, "ac"); err != nil {
				return err
			}
			if err := exclusive(notes, notesFile, "notes"); err != nil {
				return err
			}
			if err := existingFiles(descriptionFile, acFile, notesFile); err != nil {
				return err
			}
			if strings.TrimSpace(parent) != "" {
				if _, err := normalizeIssueRef(parent); err != nil {
					return err
				}
			}
			return arrives("issue create")
		},
	}
}

func (rt *runtime) cmdIssueUpdate() *Command {
	var title, typ, status, priority, parent, assignee, project string
	var description, descriptionFile, ac, acFile, notes, notesFile string
	var closeNote, closeNoteFile string
	var addTag, removeTag []string
	var dryRun bool
	return &Command{
		Name:    "update",
		Short:   "Update an issue",
		Use:     "issue update <ref> [flags]",
		minArgs: 1,
		maxArgs: 1,
		addFlags: func(fs *flagSet) {
			fs.string(&title, "title", 0, "new title")
			fs.string(&typ, "type", 0, "new type")
			fs.string(&status, "status", 0, "new status")
			fs.string(&priority, "priority", 0, "new priority")
			fs.string(&parent, "parent", 0, "new parent key or id:<n>")
			fs.string(&assignee, "assignee", 0, "new assignee id")
			fs.string(&project, "project", 0, "move to this project key")
			fs.string(&description, "description", 0, "inline description")
			fs.string(&descriptionFile, "description-file", 0, "description file, or - for stdin")
			fs.string(&ac, "ac", 0, "inline acceptance criteria")
			fs.string(&acFile, "ac-file", 0, "acceptance criteria file")
			fs.string(&notes, "notes", 0, "inline notes")
			fs.string(&notesFile, "notes-file", 0, "notes file")
			fs.string(&closeNote, "close-note", 0, "close note")
			fs.string(&closeNoteFile, "close-note-file", 0, "close note file")
			fs.strings(&addTag, "add-tag", "tag to add (repeatable)")
			fs.strings(&removeTag, "remove-tag", "tag to remove (repeatable)")
			fs.bool(&dryRun, "dry-run", 0, "accepted; the API is not available yet")
		},
		run: func(args []string) error {
			if _, err := normalizeIssueRef(args[0]); err != nil {
				return err
			}
			if strings.TrimSpace(parent) != "" {
				if _, err := normalizeIssueRef(parent); err != nil {
					return err
				}
			}
			if err := exclusive(description, descriptionFile, "description"); err != nil {
				return err
			}
			if err := exclusive(ac, acFile, "ac"); err != nil {
				return err
			}
			if err := exclusive(notes, notesFile, "notes"); err != nil {
				return err
			}
			if err := exclusive(closeNote, closeNoteFile, "close-note"); err != nil {
				return err
			}
			if err := existingFiles(descriptionFile, acFile, notesFile, closeNoteFile); err != nil {
				return err
			}
			changed := strings.TrimSpace(title+typ+status+priority+parent+assignee+project+description+descriptionFile+ac+acFile+notes+notesFile+closeNote+closeNoteFile) != "" ||
				len(addTag) > 0 || len(removeTag) > 0
			if !changed {
				return usagef("nothing to update")
			}
			return arrives("issue update")
		},
	}
}

func (rt *runtime) cmdIssueComment() *Command {
	var body, bodyFile string
	return &Command{
		Name:    "comment",
		Short:   "Comment on an issue",
		Use:     "issue comment <ref> (--body TEXT | --body-file PATH)",
		minArgs: 1,
		maxArgs: 1,
		addFlags: func(fs *flagSet) {
			fs.string(&body, "body", 0, "inline comment")
			fs.string(&bodyFile, "body-file", 0, "comment file, or - for stdin")
		},
		run: func(args []string) error {
			if _, err := normalizeIssueRef(args[0]); err != nil {
				return err
			}
			if err := exclusive(body, bodyFile, "body"); err != nil {
				return err
			}
			if strings.TrimSpace(body) == "" && strings.TrimSpace(bodyFile) == "" {
				return usagef("--body or --body-file is required")
			}
			if err := existingFiles(bodyFile); err != nil {
				return err
			}
			return arrives("issue comment")
		},
	}
}

func (rt *runtime) cmdKnowledge() *Command {
	return &Command{
		Name:  "knowledge",
		Short: "Knowledge entries",
		Use:   "knowledge <command>",
		subs: []*Command{
			rt.cmdKnowledgeList(),
			rt.cmdKnowledgeGet(),
			rt.cmdKnowledgeCreate(),
			rt.cmdKnowledgeUpdate(),
		},
	}
}

func (rt *runtime) cmdKnowledgeList() *Command {
	var project, typ string
	return &Command{
		Name:  "list",
		Short: "List knowledge entries",
		Use:   "knowledge list --project KEY [--type TYPE]",
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 0, "project key (required)")
			fs.string(&typ, "type", 0, "memory, runbook, guideline, external-system, or related-project")
		},
		run: func(args []string) error {
			if strings.TrimSpace(project) == "" {
				return usagef("--project is required")
			}
			if err := optionalKnowledgeType(typ); err != nil {
				return err
			}
			return arrives("knowledge list")
		},
	}
}

func (rt *runtime) cmdKnowledgeGet() *Command {
	var project string
	return &Command{
		Name:    "get",
		Short:   "Fetch one knowledge entry",
		Use:     "knowledge get <type> <slug> --project KEY",
		minArgs: 2,
		maxArgs: 2,
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 0, "project key (required)")
		},
		run: func(args []string) error {
			if err := requireKnowledgeType(args[0]); err != nil {
				return err
			}
			if strings.TrimSpace(args[1]) == "" {
				return usagef("<slug> is required")
			}
			if strings.TrimSpace(project) == "" {
				return usagef("--project is required")
			}
			return arrives("knowledge get")
		},
	}
}

func (rt *runtime) cmdKnowledgeCreate() *Command {
	var project, typ, slug, title, body, bodyFile, status string
	return &Command{
		Name:  "create",
		Short: "Create a knowledge entry",
		Use:   "knowledge create --type TYPE --slug SLUG --project KEY --title TITLE",
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 0, "project key (required)")
			fs.string(&typ, "type", 0, "knowledge type (required)")
			fs.string(&slug, "slug", 0, "slug (required)")
			fs.string(&title, "title", 0, "title (required)")
			fs.string(&body, "body", 0, "inline body")
			fs.string(&bodyFile, "body-file", 0, "body file, or - for stdin")
			fs.string(&status, "status", 0, "initial status")
		},
		run: func(args []string) error {
			if strings.TrimSpace(project) == "" {
				return usagef("--project is required")
			}
			if err := requireKnowledgeType(typ); err != nil {
				return err
			}
			if strings.TrimSpace(slug) == "" {
				return usagef("--slug is required")
			}
			if strings.TrimSpace(title) == "" {
				return usagef("--title is required")
			}
			if err := exclusive(body, bodyFile, "body"); err != nil {
				return err
			}
			if err := existingFiles(bodyFile); err != nil {
				return err
			}
			return arrives("knowledge create")
		},
	}
}

func (rt *runtime) cmdKnowledgeUpdate() *Command {
	var project, title, body, bodyFile, status, newSlug, metadata, metadataFile string
	return &Command{
		Name:    "update",
		Short:   "Update a knowledge entry",
		Use:     "knowledge update <type> <slug> --project KEY [flags]",
		minArgs: 2,
		maxArgs: 2,
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 0, "project key (required)")
			fs.string(&title, "title", 0, "new title")
			fs.string(&body, "body", 0, "inline body")
			fs.string(&bodyFile, "body-file", 0, "body file, or - for stdin")
			fs.string(&status, "status", 0, "new status")
			fs.string(&newSlug, "slug", 0, "rename to this slug")
			fs.string(&metadata, "metadata", 0, "inline JSON metadata")
			fs.string(&metadataFile, "metadata-file", 0, "metadata JSON file")
		},
		run: func(args []string) error {
			if err := requireKnowledgeType(args[0]); err != nil {
				return err
			}
			if strings.TrimSpace(args[1]) == "" {
				return usagef("<slug> is required")
			}
			if strings.TrimSpace(project) == "" {
				return usagef("--project is required")
			}
			if err := exclusive(body, bodyFile, "body"); err != nil {
				return err
			}
			if err := exclusive(metadata, metadataFile, "metadata"); err != nil {
				return err
			}
			if err := existingFiles(bodyFile, metadataFile); err != nil {
				return err
			}
			if strings.TrimSpace(title+body+bodyFile+status+newSlug+metadata+metadataFile) == "" {
				return usagef("nothing to update")
			}
			return arrives("knowledge update")
		},
	}
}

func (rt *runtime) cmdSearch(use string) *Command {
	var project, typ string
	var limit int
	name := "search"
	if strings.TrimSpace(use) == "" {
		use = "search"
	}
	use += " <query>"
	return &Command{
		Name:    name,
		Short:   "Search issues by free text",
		Use:     use,
		minArgs: 1,
		maxArgs: -1,
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 'p', "filter by project key")
			fs.string(&typ, "type", 0, "filter by issue type")
			fs.int(&limit, "limit", "page size")
		},
		run: func(args []string) error {
			if strings.TrimSpace(strings.Join(args, " ")) == "" {
				return usagef("search query must not be empty")
			}
			if limit < 0 {
				return usagef("--limit must be 0 or greater")
			}
			return arrives("search")
		},
	}
}

func (rt *runtime) cmdModel() *Command {
	return &Command{
		Name:  "model",
		Short: "Model roles",
		Use:   "model <resolve>",
		subs: []*Command{
			rt.cmdModelResolve(),
		},
	}
}

func (rt *runtime) cmdModelResolve() *Command {
	var author, harness, workspace string
	return &Command{
		Name:    "resolve",
		Short:   "Resolve a model role",
		Use:     "model resolve <role>",
		minArgs: 1,
		maxArgs: 1,
		addFlags: func(fs *flagSet) {
			fs.string(&author, "author-family", 0, "author model family; required for review-gate once this exists")
			fs.string(&harness, "harness", 0, "restrict to codex, claude, pi, or cursor")
			fs.string(&workspace, "workspace", 0, "workspace path")
		},
		run: func(args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return usagef("role is required")
			}
			return notYet("model resolve is not yet available")
		},
	}
}

func (rt *runtime) cmdOnboard() *Command {
	var project, agent, format string
	return &Command{
		Name:  "onboard",
		Short: "Project briefing",
		Use:   "onboard --project KEY",
		addFlags: func(fs *flagSet) {
			fs.string(&project, "project", 0, "project key (required)")
			fs.string(&agent, "agent", 0, "agent name")
			fs.string(&format, "format", 0, "md or html")
		},
		run: func(args []string) error {
			if strings.TrimSpace(project) == "" {
				return usagef("--project is required")
			}
			return arrives("onboard")
		},
	}
}

func normalizeIssueRef(raw string) (string, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", usagef("issue reference is required")
	}
	if strings.HasPrefix(ref, "id:") {
		id := strings.TrimSpace(strings.TrimPrefix(ref, "id:"))
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 {
			return "", usagef("id:<n> must contain a positive numeric issue id")
		}
		return strconv.FormatInt(n, 10), nil
	}
	bare := true
	for _, r := range ref {
		if r < '0' || r > '9' {
			bare = false
			break
		}
	}
	if bare {
		return "", usagef("ambiguous bare issue number %q; pass the full issue key (PROJECT-%s) or explicit internal id:%s", ref, ref, ref)
	}
	return ref, nil
}

func optionalKnowledgeType(seg string) error {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return nil
	}
	return requireKnowledgeType(seg)
}

func requireKnowledgeType(seg string) error {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return usagef("--type is required (one of: %s)", strings.Join(knowledgeTypes, ", "))
	}
	for _, valid := range knowledgeTypes {
		if seg == valid {
			return nil
		}
	}
	return usagef("unknown knowledge type %q (expected one of: %s)", seg, strings.Join(knowledgeTypes, ", "))
}

func exclusive(inline, file, name string) error {
	if strings.TrimSpace(inline) != "" && strings.TrimSpace(file) != "" {
		return usagef("--%s and --%s-file are mutually exclusive", name, name)
	}
	return nil
}

func existingFiles(paths ...string) error {
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || p == "-" {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if st.IsDir() {
			return fmt.Errorf("%s is a directory", p)
		}
	}
	return nil
}
