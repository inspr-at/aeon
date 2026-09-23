// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/httpapi"
)

func TestNewRejectsShortKey(t *testing.T) {
	_, err := New(Config{SessionKey: []byte("short")}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewDefaults(t *testing.T) {
	m, err := New(Config{SessionKey: bytes.Repeat([]byte{1}, 32)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.cfg.BootstrapTenantSlug != "inspr" {
		t.Fatalf("slug %q", m.cfg.BootstrapTenantSlug)
	}
}

func TestAttach(t *testing.T) {
	srv := &httpapi.Server{}
	m, err := Attach(srv, Config{SessionKey: bytes.Repeat([]byte{2}, 32), Env: envDev})
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.Modules) != 1 || len(srv.Middleware) != 1 {
		t.Fatalf("modules %d middleware %d", len(srv.Modules), len(srv.Middleware))
	}
	if srv.Modules[0] != m {
		t.Fatal("module not registered")
	}
}

func TestSessionCookieFlags(t *testing.T) {
	prod, err := New(Config{SessionKey: bytes.Repeat([]byte{3}, 32)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	prod.setSessionCookie(rec, "abc")
	c := rec.Result().Cookies()[0]
	if c.Name != "aeon_session" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(sessionTTL.Seconds()) || c.Path != "/" {
		t.Fatalf("prod cookie %+v", c)
	}
	dev, err := New(Config{SessionKey: bytes.Repeat([]byte{3}, 32), Env: envDev}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	dev.setSessionCookie(rec, "abc")
	if rec.Result().Cookies()[0].Secure {
		t.Fatal("dev cookie must not be Secure")
	}
}

func TestOIDCCookieSeal(t *testing.T) {
	m, err := New(Config{SessionKey: bytes.Repeat([]byte{4}, 32), Env: envDev}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	payload := oidcPayload{State: "st", Nonce: "no", Verifier: "ve", Exp: time.Now().Add(time.Minute).Unix()}
	if err := m.setOIDCCookie(rec, payload); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	got, err := m.readOIDCCookie(req)
	if err != nil || got.State != "st" || got.Nonce != "no" || got.Verifier != "ve" {
		t.Fatalf("open %+v %v", got, err)
	}
	tampered := *rec.Result().Cookies()[0]
	tampered.Value = tampered.Value[:len(tampered.Value)-2] + "aa"
	req = httptest.NewRequest(http.MethodGet, "/api/auth/callback", nil)
	req.AddCookie(&tampered)
	if _, err := m.readOIDCCookie(req); err == nil {
		t.Fatal("tampered cookie accepted")
	}
	expired := oidcPayload{State: "st", Nonce: "no", Verifier: "ve", Exp: time.Now().Add(-time.Minute).Unix()}
	raw, err := m.seal(expired)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.open(raw); err == nil {
		t.Fatal("expired cookie accepted")
	}
}

func TestPrefixCarriesTenant(t *testing.T) {
	id := "123e4567-e89b-12d3-a456-426614174000"
	prefix, err := prefixFor(id)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := tenantFromPrefix(prefix)
	if !ok || got != id {
		t.Fatalf("tenant %q ok %v", got, ok)
	}
	if _, ok := tenantFromPrefix("abcd"); ok {
		t.Fatal("short prefix accepted")
	}
}

func TestParseBearer(t *testing.T) {
	p, s, ok := parseBearer("Bearer aeon_abc_def")
	if !ok || p != "abc" || s != "def" {
		t.Fatalf("%q %q %v", p, s, ok)
	}
	if _, _, ok := parseBearer("Basic aeon_abc_def"); ok {
		t.Fatal("basic accepted")
	}
	if _, _, ok := parseBearer("Bearer nope"); ok {
		t.Fatal("non-aeon accepted")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv(envAppEnv, envDev)
	t.Setenv(envSessionKeyFile, "")
	t.Setenv(envTenantSlug, "")
	t.Setenv(envPublicURL, "https://aeon.example/")
	t.Setenv(envOIDCIssuer, "https://auth.inspr.at/")
	t.Setenv(envOIDCClientID, "public")
	t.Setenv(envAdminEmail, "admin@example.com")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SessionKey) != 32 || !cfg.Dev() || cfg.BootstrapTenantSlug != "inspr" {
		t.Fatalf("cfg env %q slug %q key %d", cfg.Env, cfg.BootstrapTenantSlug, len(cfg.SessionKey))
	}
	if cfg.PublicURL != "https://aeon.example" || cfg.OIDCIssuer != "https://auth.inspr.at" {
		t.Fatalf("urls %q %q", cfg.PublicURL, cfg.OIDCIssuer)
	}

	t.Setenv(envAppEnv, "prod")
	t.Setenv(envSessionKeyFile, "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("prod without key file")
	}

	dir := t.TempDir()
	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envSessionKeyFile, short)
	if _, err := FromEnv(); err == nil {
		t.Fatal("short key accepted")
	}
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, bytes.Repeat([]byte{9}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envSessionKeyFile, good)
	t.Setenv(envTenantSlug, "studio")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SessionKey) != 32 || cfg.Dev() || cfg.BootstrapTenantSlug != "studio" {
		t.Fatalf("prod cfg env %q slug %q key %d", cfg.Env, cfg.BootstrapTenantSlug, len(cfg.SessionKey))
	}
}
