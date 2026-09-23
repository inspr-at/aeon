// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportPaimosDryRunNeedsNoDatabase(t *testing.T) {
	t.Setenv("AEON_DATABASE_URL", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("source request not read-only or unauthenticated")
			w.WriteHeader(400)
			return
		}
		switch r.URL.RequestURI() {
		case "/api/projects?status=all":
			_, _ = w.Write([]byte(`[{"id":1,"key":"PAI","name":"Paimos","status":"active"}]`))
		case "/api/projects?status=deleted", "/api/users", "/api/users?status=deleted", "/api/projects/1/issues", "/api/projects/1/knowledge", "/api/issues/trash":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected source request %s", r.URL.RequestURI())
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(file, []byte("test-key"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := importPaimos([]string{"--source-url", server.URL, "--api-key-file", file, "--tenant", "test", "--project", "PAI", "--dry-run"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"projects":1`) {
		t.Fatalf("missing project count: %s", out.String())
	}
	if strings.Contains(out.String(), "test-key") {
		t.Fatal("key appeared in report")
	}
}
