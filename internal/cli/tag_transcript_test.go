// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const tagTranscriptID = "66666666-6666-4666-8666-666666666666"

func TestTagAssignedCompatTranscripts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		method string
		body   map[string]any
		output string
	}{
		{name: "rename and fields", args: []string{"--json", "tag", "update", tagTranscriptID, "--name", "New", "--color", "green", "--description", "renamed"}, method: http.MethodPatch,
			body:   map[string]any{"name": "New", "color": "green", "description": "renamed"},
			output: `{"id":"` + tagTranscriptID + `","name":"New","color":"green","description":"renamed","system":false,"created_at":"2026-09-01T00:00:00Z"}`},
		{name: "rename text", args: []string{"tag", "update", tagTranscriptID, "--name", "New"}, method: http.MethodPatch,
			body: map[string]any{"name": "New"}, output: "✓ updated tag New (#" + tagTranscriptID + ")"},
		{name: "delete confirmed", args: []string{"--json", "tag", "delete", tagTranscriptID, "--yes"}, method: http.MethodDelete,
			output: `{"action":"delete","ok":true,"tag":"Old","tag_id":"` + tagTranscriptID + `"}`},
		{name: "delete text", args: []string{"tag", "delete", tagTranscriptID, "--yes"}, method: http.MethodDelete,
			output: "✓ deleted tag Old (#" + tagTranscriptID + ")"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			var calls []transcriptRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if r.ContentLength > 0 {
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
					}
				}
				calls = append(calls, transcriptRequest{method: r.Method, path: r.URL.String(), body: body})
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/nodes/"+tagTranscriptID:
					_, _ = w.Write([]byte(`{"id":"` + tagTranscriptID + `","kind_id":"tag-kind","title":"Old","body":"old","fields":{"color":"blue"},"created_at":"2026-09-01T00:00:00Z"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/api/kinds":
					_, _ = w.Write([]byte(`{"items":[{"id":"tag-kind","slug":"tag"}]}`))
				case r.Method == http.MethodPatch && r.URL.Path == "/api/tags/"+tagTranscriptID:
					_, _ = w.Write([]byte(`{"id":"` + tagTranscriptID + `","kind_id":"tag-kind","title":"New","body":"renamed","fields":{"color":"green"},"created_at":"2026-09-01T00:00:00Z"}`))
				case r.Method == http.MethodDelete && r.URL.Path == "/api/tags/"+tagTranscriptID:
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			t.Setenv("PAIMOS_URL", srv.URL)
			t.Setenv("PAIMOS_API_KEY", testKey)
			args := append([]string{"paimos", "--config", filepath.Join(t.TempDir(), "missing")}, tc.args...)
			code, out, stderr := runCLI(args, "")
			if code != 0 || stderr != "" || strings.TrimSpace(out) != tc.output {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, out, stderr)
			}
			paths := make([]string, 0, len(calls))
			for _, call := range calls {
				paths = append(paths, call.method+" "+call.path)
			}
			want := []string{"GET /api/nodes/" + tagTranscriptID, "GET /api/kinds", tc.method + " /api/tags/" + tagTranscriptID}
			if !reflect.DeepEqual(paths, want) || !reflect.DeepEqual(calls[2].body, tc.body) {
				t.Fatalf("requests %+v, want %v and body %+v", calls, want, tc.body)
			}
		})
	}
}
