// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("AEON_DATABASE_URL", "postgres://example")
	t.Setenv("AEON_ADDR", "")
	t.Setenv("AEON_ENV", "")
	t.Setenv("AEON_PUBLIC_URL", "")
	t.Setenv("AEON_WEB_DIR", "")
	t.Setenv("AEON_BOOTSTRAP_TENANT_SLUG", "")
	t.Setenv("AEON_BOOTSTRAP_TENANT_NAME", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.Env != "dev" {
		t.Fatalf("addr/env = %s %s", cfg.Addr, cfg.Env)
	}
	if cfg.BootstrapTenantSlug != "inspr" || cfg.BootstrapTenantName != "INSPR" {
		t.Fatalf("bootstrap = %s %s", cfg.BootstrapTenantSlug, cfg.BootstrapTenantName)
	}
	if cfg.DatabaseURL != "postgres://example" || cfg.PublicURL != "" || cfg.WebDir != "" {
		t.Fatalf("unexpected cfg %+v", cfg)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("AEON_DATABASE_URL", "postgres://example")
	t.Setenv("AEON_ADDR", ":9090")
	t.Setenv("AEON_ENV", "prod")
	t.Setenv("AEON_PUBLIC_URL", "https://aeon.example")
	t.Setenv("AEON_WEB_DIR", "/var/aeon/web")
	t.Setenv("AEON_BOOTSTRAP_TENANT_SLUG", "studio")
	t.Setenv("AEON_BOOTSTRAP_TENANT_NAME", "Studio")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9090" || cfg.Env != "prod" || cfg.PublicURL != "https://aeon.example" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.WebDir != "/var/aeon/web" || cfg.BootstrapTenantSlug != "studio" || cfg.BootstrapTenantName != "Studio" {
		t.Fatalf("%+v", cfg)
	}
}

func TestFromEnvRejects(t *testing.T) {
	t.Setenv("AEON_DATABASE_URL", "")
	t.Setenv("AEON_ENV", "dev")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected missing database url to fail")
	}

	t.Setenv("AEON_DATABASE_URL", "postgres://example")
	t.Setenv("AEON_ENV", "staging")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected invalid env to fail")
	}
}
