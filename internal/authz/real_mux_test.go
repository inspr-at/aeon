// SPDX-License-Identifier: AGPL-3.0-only

package authz_test

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/activity"
	"github.com/inspr-at/aeon/internal/agentaccounts"
	"github.com/inspr-at/aeon/internal/agentruns"
	"github.com/inspr-at/aeon/internal/approvals"
	"github.com/inspr-at/aeon/internal/attachments"
	"github.com/inspr-at/aeon/internal/auth"
	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/business/costunits"
	"github.com/inspr-at/aeon/internal/business/crm"
	"github.com/inspr-at/aeon/internal/business/directory"
	"github.com/inspr-at/aeon/internal/business/hours"
	"github.com/inspr-at/aeon/internal/business/quotes"
	"github.com/inspr-at/aeon/internal/business/quotes/collaboration"
	"github.com/inspr-at/aeon/internal/business/quotes/confirmation"
	publicquotes "github.com/inspr-at/aeon/internal/business/quotes/public"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/fromclassic"
	"github.com/inspr-at/aeon/internal/greetings"
	"github.com/inspr-at/aeon/internal/harness"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/imports"
	"github.com/inspr-at/aeon/internal/inbox"
	"github.com/inspr-at/aeon/internal/intake"
	"github.com/inspr-at/aeon/internal/journey"
	"github.com/inspr-at/aeon/internal/knowledge"
	"github.com/inspr-at/aeon/internal/modelregistry"
	"github.com/inspr-at/aeon/internal/nodes"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/profile"
	"github.com/inspr-at/aeon/internal/projectgroups"
	"github.com/inspr-at/aeon/internal/relations"
	"github.com/inspr-at/aeon/internal/releasehistory"
	"github.com/inspr-at/aeon/internal/releases"
	"github.com/inspr-at/aeon/internal/requirements"
	"github.com/inspr-at/aeon/internal/search"
	"github.com/inspr-at/aeon/internal/stagehandoff"
	"github.com/inspr-at/aeon/internal/views"
	"github.com/inspr-at/aeon/internal/workorders"
)

// This mirrors the coordinator's production module list at the ServeMux
// boundary. Constructors with runtime secrets are represented by their module
// values; Mount itself only registers paths. The source walk catches newly
// registered paths even before the route declaration table is updated.
func TestRealMuxRouteCoverage(t *testing.T) {
	authModule, err := auth.New(auth.Config{Env: "dev", SessionKey: make([]byte, 32)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	messaging, err := inbox.NewMessaging(nil, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	modules := []httpapi.Module{
		authModule, authz.New(nil), nodes.New(nil, nil), fromclassic.New(nil), relations.New(nil),
		events.New(nil), search.New(nil, nil), views.New(nil), activity.New(nil),
		attachments.New(nil, attachments.Store{}), &greetings.Module{}, knowledge.New(nil),
		projectgroups.New(nil), &releasehistory.Module{}, profile.New(nil, attachments.Store{}),
		imports.New(nil), inbox.New(nil), messaging, harness.New(nil), workorders.New(nil),
		agentruns.New(nil), approvals.New(nil), modelregistry.New(nil), agentaccounts.New(nil),
		journey.New(nil), requirements.New(nil), releases.New(nil), intake.New(nil),
		plugins.New(nil), stagehandoff.New(nil, nil), costunits.New(nil, nil), crm.New(nil, nil),
		&quotes.Module{}, &collaboration.Module{}, &publicquotes.Module{}, &confirmation.Module{},
		hours.New(nil, nil), directory.New(nil, nil),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /api/version", func(http.ResponseWriter, *http.Request) {})
	for _, module := range modules {
		module.Mount(mux)
	}
	patternRE := regexp.MustCompile(`"((?:GET|POST|PUT|PATCH|DELETE|HEAD) /api/[^"\n]+)"`)
	variableRE := regexp.MustCompile(`\{[^}]+\}`)
	seen := map[string]bool{}
	err = filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "route_map.go" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range patternRE.FindAllSubmatch(body, -1) {
			declaration := string(match[1])
			if seen[declaration] {
				continue
			}
			seen[declaration] = true
			method, route, _ := strings.Cut(declaration, " ")
			req := httptest.NewRequest(method, variableRE.ReplaceAllString(route, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), nil)
			_, matched := mux.Handler(req)
			if matched != declaration {
				t.Errorf("%s: real mux matched %q", declaration, matched)
			}
			if _, declared := authz.PermissionForPattern(matched); !declared {
				t.Errorf("%s: no permission declared", declaration)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
