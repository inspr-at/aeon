// SPDX-License-Identifier: AGPL-3.0-only

// Package config reads the aeon serve process configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config is the process configuration for `aeon serve`.
type Config struct {
	Addr                string
	DatabaseURL         string
	Env                 string // "dev" or "prod"
	PublicURL           string
	WebDir              string
	BootstrapTenantSlug string
	BootstrapTenantName string
}

// FromEnv reads AEON_* variables. Empty optional values take their defaults.
// AEON_DATABASE_URL is required. AEON_ENV must be dev or prod.
func FromEnv() (Config, error) {
	cfg := Config{
		Addr:                getenv("AEON_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("AEON_DATABASE_URL"),
		Env:                 getenv("AEON_ENV", "dev"),
		PublicURL:           os.Getenv("AEON_PUBLIC_URL"),
		WebDir:              os.Getenv("AEON_WEB_DIR"),
		BootstrapTenantSlug: getenv("AEON_BOOTSTRAP_TENANT_SLUG", "inspr"),
		BootstrapTenantName: getenv("AEON_BOOTSTRAP_TENANT_NAME", "INSPR"),
	}
	switch cfg.Env {
	case "dev", "prod":
	default:
		return Config{}, fmt.Errorf("AEON_ENV must be dev or prod, got %q", cfg.Env)
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("AEON_DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
