// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/inspr-at/aeon/internal/brand"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/version"
)

type moduleFunc func(*http.ServeMux)

func (f moduleFunc) Mount(mux *http.ServeMux) { f(mux) }

func TestHandlersAndMiddleware(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/api/version", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var body versionBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Version != version.Version || body.Scheme != version.Scheme || body.Brand != brand.Default() {
			t.Fatalf("%+v", body)
		}
		// A deployment's brand replaces the embedded one.
		custom := brand.Brand{Schema: brand.Schema, Product: "NOVA", Generation: "1", ReleaseName: "DAWN", Wordmark: "NOVA DAWN", ShortName: "DAWN"}
		var branded versionBody
		if err := json.Unmarshal(get(t, (&Server{Brand: &custom}).Handler(), "/api/version", "").Body.Bytes(), &branded); err != nil || branded.Brand != custom {
			t.Fatalf("custom brand %+v %v", branded, err)
		}
		if rec.Header().Get("Content-Security-Policy") != "default-src 'self'; img-src 'self' blob: data:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'" {
			t.Fatalf("csp %q", rec.Header().Get("Content-Security-Policy"))
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff")
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Permissions-Policy") == "" {
			t.Fatal("missing browser isolation headers")
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing request id")
		}
	})

	t.Run("request id generated", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/api/version", "trace-1")
		if rec.Header().Get("X-Request-ID") == "trace-1" || rec.Header().Get("X-Request-ID") == "" {
			t.Fatalf("id %q", rec.Header().Get("X-Request-ID"))
		}
	})

	t.Run("request id rejected", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/api/version", "bad\nid")
		if strings.Contains(rec.Header().Get("X-Request-ID"), "\n") || rec.Header().Get("X-Request-ID") == "bad\nid" {
			t.Fatalf("id %q", rec.Header().Get("X-Request-ID"))
		}
	})

	t.Run("health without pool", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/api/health", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var body healthBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != "ok" || body.DB != "down" {
			t.Fatalf("%+v", body)
		}
	})

	t.Run("placeholder", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "<title>"+brand.Default().Wordmark+"</title>") {
			t.Fatalf("body %s", rec.Body.String())
		}
		if rec.Header().Get("Content-Security-Policy") != "default-src 'self'; img-src 'self' blob: data:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'" {
			t.Fatal("csp missing on placeholder")
		}
		// The deployment's brand names the placeholder too.
		custom := brand.Brand{Schema: brand.Schema, Product: "NOVA", Generation: "1", ReleaseName: "DAWN", Wordmark: "NOVA DAWN", ShortName: "DAWN"}
		if body := get(t, (&Server{Brand: &custom}).Handler(), "/", "").Body.String(); !strings.Contains(body, "<title>NOVA DAWN</title>") || strings.Contains(body, "AEON") {
			t.Fatalf("branded placeholder %s", body)
		}
	})

	t.Run("unknown api", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/api/missing", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"error"`) {
			t.Fatalf("body %s", rec.Body.String())
		}
	})

	t.Run("middleware and modules", func(t *testing.T) {
		var order []string
		s := &Server{
			Middleware: []func(http.Handler) http.Handler{
				func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						order = append(order, "outer")
						next.ServeHTTP(w, r)
					})
				},
				func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						order = append(order, "inner")
						w.Header().Set("X-From-MW", "1")
						next.ServeHTTP(w, r)
					})
				},
			},
			Modules: []Module{moduleFunc(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
					order = append(order, "handler")
					w.WriteHeader(http.StatusNoContent)
				})
			})},
		}
		h := s.Handler()
		rec := get(t, h, "/api/ping", "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status %d", rec.Code)
		}
		if strings.Join(order, ",") != "outer,inner,handler" {
			t.Fatalf("order %v", order)
		}
		if rec.Header().Get("X-From-MW") != "1" {
			t.Fatal("middleware did not run on /api")
		}
		order = nil
		page := get(t, h, "/", "")
		if page.Header().Get("X-From-MW") != "" || len(order) != 0 {
			t.Fatalf("middleware ran outside /api: header %q order %v", page.Header().Get("X-From-MW"), order)
		}
	})

	t.Run("panic", func(t *testing.T) {
		s := &Server{Modules: []Module{moduleFunc(func(mux *http.ServeMux) {
			mux.HandleFunc("GET /api/boom", func(http.ResponseWriter, *http.Request) {
				panic("boom")
			})
		})}}
		rec := get(t, s.Handler(), "/api/boom", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"internal error"`) {
			t.Fatalf("body %s", rec.Body.String())
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("csp missing on panic")
		}
	})

	t.Run("spa", func(t *testing.T) {
		s := &Server{Web: fstest.MapFS{
			"index.html":    {Data: []byte("<!doctype html><title>App</title>")},
			"assets/app.js": {Data: []byte("console.log(1)")},
		}}
		h := s.Handler()
		asset := get(t, h, "/assets/app.js", "")
		if asset.Body.String() != "console.log(1)" {
			t.Fatalf("asset %s", asset.Body.String())
		}
		fallback := get(t, h, "/nodes/1", "")
		if !strings.Contains(fallback.Body.String(), "<title>App</title>") {
			t.Fatalf("fallback %s", fallback.Body.String())
		}
		api := get(t, h, "/api/version", "")
		if !strings.Contains(api.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("api content type %s", api.Header().Get("Content-Type"))
		}
	})
}

func TestHealthDatabase(t *testing.T) {
	pool := dbtest.Open(t).Admin
	rec := get(t, (&Server{Pool: pool}).Handler(), "/api/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body healthBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.DB != "ok" {
		t.Fatalf("%+v", body)
	}
}

func get(t *testing.T, h http.Handler, path, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCapabilityAndPanicDetailsNeverEnterRequestLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	token := strings.Repeat("A", 43)
	secretPanic := "upstream-credential-must-stay-private"
	s := &Server{Modules: []Module{moduleFunc(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/public/quotes/{selector}/{token}", func(http.ResponseWriter, *http.Request) {
			panic(secretPanic)
		})
	})}}
	rec := get(t, s.Handler(), "/api/public/quotes/selector/"+token, token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), secretPanic) {
		t.Fatal("capability path or panic value entered request logs")
	}
}

func TestRequestLogRoutes(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	s := &Server{
		Middleware: []func(http.Handler) http.Handler{func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Auth middleware may clone an authenticated request.
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, "inner")))
			})
		}},
		Modules: []Module{moduleFunc(func(mux *http.ServeMux) {
			for _, pattern := range []string{
				"GET /api/nodes/{id}",
				"GET /api/public/quotes/{tenant}/{token}",
				"POST /api/public/quotes/{tenant}/{token}/accept",
			} {
				mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				})
			}
		})},
	}
	server := s.Handler()
	bare := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	tests := []struct {
		name, method, path, token, wantRoute string
		handler                              http.Handler
		wantStatus                           int
	}{
		{"matched node", "GET", "/api/nodes/123?access=query-secret-node", "query-secret-node", "GET /api/nodes/{id}", server, http.StatusNoContent},
		{"matched public quote", "GET", "/api/public/quotes/tenant/SECRET-QUOTE-TOKEN?access=query-secret-quote", "SECRET-QUOTE-TOKEN", "GET /api/public/quotes/{tenant}/{token}", server, http.StatusNoContent},
		{"matched public acceptance", "POST", "/api/public/quotes/tenant/SECRET-ACCEPT-TOKEN/accept", "SECRET-ACCEPT-TOKEN", "POST /api/public/quotes/{tenant}/{token}/accept", server, http.StatusNoContent},
		{"unmatched public quote", "GET", "/api/public/quotes/tenant/SECRET-UNKNOWN-TOKEN/extra", "SECRET-UNKNOWN-TOKEN", "/api/public/{redacted}", server, http.StatusNotFound},
		{"spa q link", "GET", "/q/SECRET-Q-TOKEN", "SECRET-Q-TOKEN", "/q/{redacted}", server, http.StatusOK},
		{"spa offer link", "GET", "/offers/tenant/SECRET-OFFER-TOKEN", "SECRET-OFFER-TOKEN", "/offers/{redacted}", server, http.StatusOK},
		{"invite token", "GET", "/api/invites/SECRET-INVITE-TOKEN/accept", "SECRET-INVITE-TOKEN", "/api/invites/{redacted}", server, http.StatusNotFound},
		{"link token", "GET", "/api/links/SECRET-LINK-TOKEN", "SECRET-LINK-TOKEN", "/api/links/{redacted}", server, http.StatusNotFound},
		{"confirmation token", "GET", "/api/quotes/123/confirmation/SECRET-CONFIRM-TOKEN", "SECRET-CONFIRM-TOKEN", "/api/quotes/{redacted}", server, http.StatusNotFound},
		{"attachment download token", "GET", "/api/attachments/123/content/SECRET-DOWNLOAD-TOKEN", "SECRET-DOWNLOAD-TOKEN", "/api/attachments/{redacted}", server, http.StatusNotFound},
		{"encoded public token", "GET", "/api/public/quotes/tenant/SECRET%2FENCODED-TOKEN", "SECRET/ENCODED-TOKEN", "GET /api/public/quotes/{tenant}/{token}", server, http.StatusNoContent},
		{"unmatched unknown token", "GET", "/api/SECRET-UNKNOWN-SECTION/other", "SECRET-UNKNOWN-SECTION", "/api/{redacted}", server, http.StatusNotFound},
		{"no mux ordinary path", "GET", "/api/nodes/123", "", "/api/nodes/{redacted}", bare, http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d", rec.Code, tt.wantStatus)
			}
			log := logs.String()
			if tt.token != "" && strings.Contains(log, tt.token) {
				t.Fatal("token entered request log")
			}
			if strings.Contains(log, "SECRET%2FENCODED-TOKEN") {
				t.Fatal("encoded token entered request log")
			}
			if strings.Contains(log, "query-secret-") {
				t.Fatal("query string entered request log")
			}
			var entry struct {
				Method     string `json:"method"`
				Path       string `json:"path"`
				Status     int    `json:"status"`
				RequestID  string `json:"request_id"`
				DurationMS int64  `json:"duration_ms"`
			}
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Path != tt.wantRoute || entry.Method != tt.method || entry.Status != tt.wantStatus {
				t.Fatalf("unexpected request log: path %q, method %q, status %d", entry.Path, entry.Method, entry.Status)
			}
			if entry.RequestID == "" || entry.RequestID != rec.Header().Get(requestIDHeader) || entry.DurationMS < 0 {
				t.Fatal("request id or duration missing from request log")
			}
		})
	}
}
