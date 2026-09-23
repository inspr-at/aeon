// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAnchorIndexCommentFormsAndSymbol(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"backend/handlers/context.go":        "// @paimos PAI-68 \"anchor ingest endpoint\"\nfunc x() {}\n",
		"frontend/src/components/example.ts": "// @paimos PAI-65 \"scanner command\"\nexport const x = 1\n",
		"schema/example.sql":                 "-- @paimos PAI-79 \"blast radius fixture\"\nselect 1;\n",
		"docs/example.md":                    "<!-- @paimos PAI-81 \"agent onboarding\" -->\n",
		"config/example.yaml":                "# @paimos PAI-72 \"manifest mirror\"\nkey: value\n",
		"backend/handlers/symbol.go": `package handlers

func RetrieveProjectContext() {
	// @paimos PAI-80 "retrieve endpoint"
	println("ok")
}
`,
		"frontend/src/context.ts": `export function buildContext() {
  // @paimos PAI-78 "symbol extraction"
  return true
}
`,
	}
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index, err := buildAnchorIndex(root, "paimos-app", "1")
	if err != nil {
		t.Fatal(err)
	}
	if index.Repo != "paimos-app" || index.SchemaVersion != "1" || index.GeneratedAt == "" {
		t.Fatalf("index header %+v", index)
	}
	for _, issueKey := range []string{"PAI-68", "PAI-65", "PAI-79", "PAI-81", "PAI-72", "PAI-80", "PAI-78"} {
		if got := len(index.Anchors[issueKey]); got != 1 {
			t.Fatalf("%s anchors: got %d want 1", issueKey, got)
		}
	}
	if index.Anchors["PAI-72"][0].Confidence != "declared" {
		t.Fatalf("confidence %q", index.Anchors["PAI-72"][0].Confidence)
	}
	if index.Anchors["PAI-81"][0].Symbol != nil {
		t.Fatalf("markdown symbol %#v", index.Anchors["PAI-81"][0].Symbol)
	}
	gotGo, ok := index.Anchors["PAI-80"][0].Symbol.(anchorSymbol)
	if !ok || gotGo.Name != "RetrieveProjectContext" || gotGo.Kind != "function" || gotGo.Language != "go" {
		t.Fatalf("go symbol %#v", index.Anchors["PAI-80"][0].Symbol)
	}
	gotTS, ok := index.Anchors["PAI-78"][0].Symbol.(anchorSymbol)
	if !ok || gotTS.Name != "buildContext" || gotTS.Kind != "function" || gotTS.Language != "typescript" {
		t.Fatalf("ts symbol %#v", index.Anchors["PAI-78"][0].Symbol)
	}
	raw, err := json.Marshal(index.Anchors["PAI-81"][0])
	if err != nil || !strings.Contains(string(raw), `"symbol":null`) {
		t.Fatalf("null symbol shape %s %v", raw, err)
	}
}

func TestCompareAnchorIndexesLineDriftAndMissing(t *testing.T) {
	expected := &anchorIndex{Anchors: map[string][]anchorRecord{
		"PAI-65": {{File: "a.go", Line: 10, Label: "scan", Confidence: "declared"}},
		"PAI-68": {{File: "b.go", Line: 20, Label: "upload", Confidence: "declared"}},
	}}
	current := &anchorIndex{Anchors: map[string][]anchorRecord{
		"PAI-65": {{File: "a.go", Line: 14, Label: "scan", Confidence: "declared"}},
	}}
	report := compareAnchorIndexes(expected, current)
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "line drift") {
		t.Fatalf("warnings %#v", report.Warnings)
	}
	if len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "missing anchor") {
		t.Fatalf("errors %#v", report.Errors)
	}
}

func TestAnchorsScanAndVerifyCLI(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("note.go", []byte("package note\n\n// @paimos AEON-55 \"compat anchor\"\nfunc Scan() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI([]string{"paimos", "anchors", "scan", "--repo", "paimos-app", "--repo-root", root}, "")
	if code != 0 || !strings.Contains(out, "wrote .paimos/anchors.json") || errOut != "" {
		t.Fatalf("scan code %d out %q err %q", code, out, errOut)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".paimos", "anchors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index anchorIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if index.Repo != "paimos-app" || index.SchemaVersion != "1" || len(index.Anchors["AEON-55"]) != 1 {
		t.Fatalf("index %+v", index)
	}
	if index.Anchors["AEON-55"][0].File != "note.go" || index.Anchors["AEON-55"][0].Label != "compat anchor" || index.Anchors["AEON-55"][0].Confidence != "declared" {
		t.Fatalf("record %+v", index.Anchors["AEON-55"][0])
	}
	code, out, errOut = runCLI([]string{"paimos", "anchors", "verify", "--repo-root", root, "--index", filepath.Join(root, ".paimos", "anchors.json")}, "")
	if code != 0 || !strings.Contains(out, "anchor index is current") {
		t.Fatalf("verify code %d out %q err %q", code, out, errOut)
	}
	if err := os.WriteFile("note.go", []byte("package note\n\n\n// @paimos AEON-55 \"compat anchor\"\nfunc Scan() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runCLI([]string{"paimos", "anchors", "verify", "--repo-root", root, "--index", ".paimos/anchors.json"}, "")
	if code != 0 || !strings.Contains(out, "warn:") || !strings.Contains(out, "line drift") {
		t.Fatalf("drift code %d out %q err %q", code, out, errOut)
	}
	if err := os.WriteFile("note.go", []byte("package note\nfunc Scan() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runCLI([]string{"paimos", "anchors", "verify", "--repo-root", root}, "")
	if code != 1 || !strings.Contains(errOut, "missing anchor") || !strings.Contains(errOut, "anchor verification failed") {
		t.Fatalf("missing code %d out %q err %q", code, out, errOut)
	}
}
