// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Anchors are repo-side. scan writes .paimos/anchors.json; verify compares
// that index to the live comments. Neither command calls Aeon. The JSON
// shape matches the classic index: repo, schema_version, optional
// repo_revision, generated_at, and anchors keyed by issue. Line drift is a
// warning; a missing comment is an error. Symbol is null except where a
// declaration above the comment can be read without a parser.

type anchorIndex struct {
	Repo          string                    `json:"repo"`
	SchemaVersion string                    `json:"schema_version"`
	RepoRevision  string                    `json:"repo_revision,omitempty"`
	GeneratedAt   string                    `json:"generated_at"`
	Anchors       map[string][]anchorRecord `json:"anchors"`
}

type anchorRecord struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Label      string `json:"label,omitempty"`
	Confidence string `json:"confidence"`
	Symbol     any    `json:"symbol"`
}

type anchorSymbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Language  string `json:"language"`
}

type anchorVerifyReport struct {
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
}

var anchorPattern = regexp.MustCompile(`^\s*(?://|#|--|<!--)\s*@paimos\s+([A-Z][A-Z0-9]{0,15}-\d+)(?:\s+"([^"]+)")?\s*(?:-->)?\s*$`)

func (rt *runtime) cmdAnchors() *Command {
	return &Command{
		Name:  "anchors",
		Short: "Scan and verify @paimos anchors",
		Use:   "anchors <scan|verify>",
		subs:  []*Command{rt.cmdAnchorsScan(), rt.cmdAnchorsVerify()},
	}
}

func (rt *runtime) cmdAnchorsScan() *Command {
	var repoRoot, outputPath, repoName, schemaVersion string
	outputPath = ".paimos/anchors.json"
	schemaVersion = "1"
	return &Command{
		Name:  "scan",
		Short: "Scan a repository for @paimos anchors",
		Use:   "anchors scan [--output .paimos/anchors.json]",
		addFlags: func(fs *flagSet) {
			fs.string(&repoRoot, "repo-root", 0, "repository root (default: git top-level or the working directory)")
			fs.string(&outputPath, "output", 0, "index path (default .paimos/anchors.json); empty prints the index")
			fs.string(&repoName, "repo", 0, "repo identifier written into the index")
			fs.string(&schemaVersion, "schema-version", 0, "anchor index schema version")
		},
		run: func(args []string) error {
			root, err := repoRootFrom(repoRoot)
			if err != nil {
				return err
			}
			index, err := buildAnchorIndex(root, repoName, schemaVersion)
			if err != nil {
				return err
			}
			if strings.TrimSpace(outputPath) == "" {
				if rt.jsonOut {
					return rt.printJSON(index)
				}
				raw, err := json.MarshalIndent(index, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(rt.stdout, string(raw))
				return nil
			}
			if err := writeAnchorIndex(outputPath, index); err != nil {
				return err
			}
			if !rt.jsonOut {
				fmt.Fprintf(rt.stdout, "wrote %s\n", outputPath)
			}
			return nil
		},
	}
}

func (rt *runtime) cmdAnchorsVerify() *Command {
	var repoRoot, indexPath, repoName, schemaVersion string
	indexPath = ".paimos/anchors.json"
	schemaVersion = "1"
	return &Command{
		Name:  "verify",
		Short: "Verify a committed anchor index against live comments",
		Use:   "anchors verify [--index .paimos/anchors.json]",
		addFlags: func(fs *flagSet) {
			fs.string(&repoRoot, "repo-root", 0, "repository root (default: git top-level or the working directory)")
			fs.string(&indexPath, "index", 0, "path to the committed anchor index")
			fs.string(&repoName, "repo", 0, "repo identifier used for the live scan")
			fs.string(&schemaVersion, "schema-version", 0, "anchor index schema version")
		},
		run: func(args []string) error {
			root, err := repoRootFrom(repoRoot)
			if err != nil {
				return err
			}
			current, err := buildAnchorIndex(root, repoName, schemaVersion)
			if err != nil {
				return err
			}
			expected, err := readAnchorIndex(indexPath)
			if err != nil {
				return err
			}
			report := compareAnchorIndexes(expected, current)
			if rt.jsonOut {
				if err := rt.printJSON(report); err != nil {
					return err
				}
			} else if len(report.Errors) == 0 && len(report.Warnings) == 0 {
				fmt.Fprintln(rt.stdout, "anchor index is current")
			} else {
				for _, w := range report.Warnings {
					fmt.Fprintf(rt.stdout, "warn: %s\n", w)
				}
				for _, e := range report.Errors {
					fmt.Fprintf(rt.stderr, "error: %s\n", e)
				}
			}
			if len(report.Errors) > 0 {
				return &exitError{code: 1, msg: fmt.Sprintf("anchor verification failed; regenerate with `%s anchors scan --repo-root %s --output %s`", rt.program, root, indexPath)}
			}
			return nil
		},
	}
}

func buildAnchorIndex(root, repoOverride, schemaVersion string) (*anchorIndex, error) {
	files, err := listRepoFiles(root)
	if err != nil {
		return nil, err
	}
	repoName, revision, _ := detectRepoIdentity(root)
	if strings.TrimSpace(repoOverride) != "" {
		repoName = strings.TrimSpace(repoOverride)
	}
	if strings.TrimSpace(schemaVersion) == "" {
		schemaVersion = "1"
	}
	index := &anchorIndex{
		Repo:          repoName,
		SchemaVersion: strings.TrimSpace(schemaVersion),
		RepoRevision:  revision,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Anchors:       map[string][]anchorRecord{},
	}
	for _, rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		text := string(content)
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			match := anchorPattern.FindStringSubmatch(strings.TrimRight(line, "\r"))
			if len(match) == 0 {
				continue
			}
			issueKey := strings.ToUpper(strings.TrimSpace(match[1]))
			record := anchorRecord{
				File:       filepath.ToSlash(rel),
				Line:       i + 1,
				Label:      strings.TrimSpace(match[2]),
				Confidence: "declared",
				Symbol:     detectAnchorSymbol(rel, text, i+1),
			}
			index.Anchors[issueKey] = append(index.Anchors[issueKey], record)
		}
	}
	keys := make([]string, 0, len(index.Anchors))
	for k := range index.Anchors {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	sorted := make(map[string][]anchorRecord, len(index.Anchors))
	for _, k := range keys {
		records := slices.Clone(index.Anchors[k])
		slices.SortFunc(records, func(a, b anchorRecord) int {
			if a.File != b.File {
				return strings.Compare(a.File, b.File)
			}
			if a.Line != b.Line {
				return a.Line - b.Line
			}
			return strings.Compare(a.Label, b.Label)
		})
		sorted[k] = records
	}
	index.Anchors = sorted
	return index, nil
}

func writeAnchorIndex(path string, index *anchorIndex) error {
	if index == nil {
		return fmt.Errorf("anchor index is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

func readAnchorIndex(path string) (*anchorIndex, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read anchor index: %w", err)
	}
	var idx anchorIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse anchor index: %w", err)
	}
	if idx.Anchors == nil {
		idx.Anchors = map[string][]anchorRecord{}
	}
	return &idx, nil
}

func compareAnchorIndexes(expected, current *anchorIndex) anchorVerifyReport {
	report := anchorVerifyReport{Warnings: []string{}, Errors: []string{}}
	type key struct {
		issue string
		file  string
		label string
	}
	expectedMap := map[key]anchorRecord{}
	currentMap := map[key]anchorRecord{}
	if expected != nil {
		for issue, list := range expected.Anchors {
			for _, rec := range list {
				expectedMap[key{issue: issue, file: rec.File, label: rec.Label}] = rec
			}
		}
	}
	if current != nil {
		for issue, list := range current.Anchors {
			for _, rec := range list {
				currentMap[key{issue: issue, file: rec.File, label: rec.Label}] = rec
			}
		}
	}
	keys := make([]key, 0, len(expectedMap))
	for k := range expectedMap {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b key) int {
		if a.issue != b.issue {
			return strings.Compare(a.issue, b.issue)
		}
		if a.file != b.file {
			return strings.Compare(a.file, b.file)
		}
		return strings.Compare(a.label, b.label)
	})
	for _, k := range keys {
		oldRec := expectedMap[k]
		newRec, ok := currentMap[k]
		if !ok {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: missing anchor %s:%d %q", k.issue, oldRec.File, oldRec.Line, oldRec.Label))
			continue
		}
		if oldRec.Line != newRec.Line {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: line drift %s %d -> %d (regenerate .paimos/anchors.json)", k.issue, oldRec.File, oldRec.Line, newRec.Line))
		}
	}
	for k, rec := range currentMap {
		if _, ok := expectedMap[k]; !ok {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: new anchor discovered %s:%d %q", k.issue, rec.File, rec.Line, rec.Label))
		}
	}
	slices.Sort(report.Warnings)
	slices.Sort(report.Errors)
	return report
}

func repoRootFrom(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("repo root %s is not a directory", path)
		}
		return abs, nil
	}
	if out, err := gitOutput("", "rev-parse", "--show-toplevel"); err == nil && out != "" {
		return out, nil
	}
	return os.Getwd()
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func listRepoFiles(root string) ([]string, error) {
	out, err := gitOutput(root, "ls-files", "--cached", "--others", "--exclude-standard")
	if err == nil {
		lines := strings.Split(out, "\n")
		files := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			files = append(files, filepath.ToSlash(line))
		}
		slices.Sort(files)
		return files, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	return files, nil
}

func detectRepoIdentity(root string) (repoName, repoRevision, remoteURL string) {
	if head, err := gitOutput(root, "rev-parse", "HEAD"); err == nil {
		repoRevision = head
	}
	if remote, err := gitOutput(root, "remote", "get-url", "origin"); err == nil {
		remoteURL = remote
		trimmed := strings.TrimSuffix(strings.TrimSpace(remote), ".git")
		trimmed = strings.TrimRight(trimmed, "/")
		if idx := strings.LastIndex(trimmed, "/"); idx >= 0 && idx < len(trimmed)-1 {
			repoName = trimmed[idx+1:]
		}
	}
	if repoName == "" {
		repoName = filepath.Base(root)
	}
	return repoName, repoRevision, remoteURL
}

var (
	goFuncRE  = regexp.MustCompile(`^func\s+(\([^)\n]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goTypeRE  = regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	jsFuncRE  = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	jsClassRE = regexp.MustCompile(`^(?:export\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	jsIfaceRE = regexp.MustCompile(`^(?:export\s+)?interface\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	jsTypeRE  = regexp.MustCompile(`^(?:export\s+)?type\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
)

func detectAnchorSymbol(rel, content string, line int) any {
	lang := symbolLanguage(rel)
	if lang == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if line < 1 || line > len(lines) {
		return nil
	}
	for i := line - 1; i >= 0; i-- {
		name, kind, ok := matchDeclaration(lang, strings.TrimSpace(strings.TrimRight(lines[i], "\r")))
		if !ok {
			continue
		}
		end := enclosingEnd(lines, i)
		if line > end {
			continue
		}
		return anchorSymbol{Name: name, Kind: kind, StartLine: i + 1, EndLine: end, Language: lang}
	}
	return nil
}

func symbolLanguage(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go":
		return "go"
	case ".js", ".jsx":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	default:
		return ""
	}
}

func matchDeclaration(lang, line string) (name, kind string, ok bool) {
	switch lang {
	case "go":
		if m := goFuncRE.FindStringSubmatch(line); len(m) == 3 {
			kind = "function"
			if strings.TrimSpace(m[1]) != "" {
				kind = "method"
			}
			return m[2], kind, true
		}
		if m := goTypeRE.FindStringSubmatch(line); len(m) == 2 {
			return m[1], "type", true
		}
	default:
		if m := jsFuncRE.FindStringSubmatch(line); len(m) == 2 {
			return m[1], "function", true
		}
		if m := jsClassRE.FindStringSubmatch(line); len(m) == 2 {
			return m[1], "class", true
		}
		if m := jsIfaceRE.FindStringSubmatch(line); len(m) == 2 {
			return m[1], "interface", true
		}
		if m := jsTypeRE.FindStringSubmatch(line); len(m) == 2 {
			return m[1], "type", true
		}
	}
	return "", "", false
}

func enclosingEnd(lines []string, start int) int {
	depth := 0
	seen := false
	for i := start; i < len(lines); i++ {
		for _, r := range lines[i] {
			switch r {
			case '{':
				depth++
				seen = true
			case '}':
				if depth > 0 {
					depth--
				}
			}
		}
		if seen && depth == 0 {
			return i + 1
		}
	}
	if len(lines) == 0 {
		return start + 1
	}
	return len(lines)
}
