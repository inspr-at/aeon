// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"encoding/json"
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
		if rec.Header().Get("Content-Security-Policy") != "default-src 'self'; img-src 'self' blob: data:" {
			t.Fatalf("csp %q", rec.Header().Get("Content-Security-Policy"))
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff")
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing request id")
		}
	})

	t.Run("request id echoed", func(t *testing.T) {
		rec := get(t, (&Server{}).Handler(), "/api/version", "trace-1")
		if rec.Header().Get("X-Request-ID") != "trace-1" {
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
		if rec.Header().Get("Content-Security-Policy") != "default-src 'self'; img-src 'self' blob: data:" {
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
