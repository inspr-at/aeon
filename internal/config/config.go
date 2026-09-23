// SPDX-License-Identifier: AGPL-3.0-only

// Package config reads the aeon serve process configuration from the environment.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
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
// AEON_DATABASE_PASSWORD_FILE, when set, supplies the database password from a
// file (host-generated secret) so it never appears in the environment.
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
	if f := os.Getenv("AEON_DATABASE_PASSWORD_FILE"); f != "" {
		u, err := withPasswordFile(cfg.DatabaseURL, f)
		if err != nil {
			return Config{}, err
		}
		cfg.DatabaseURL = u
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// withPasswordFile sets the password of a postgres:// URL from the first line of file.
func withPasswordFile(dsn, file string) (string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("AEON_DATABASE_PASSWORD_FILE: %w", err)
	}
	pw := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	if pw == "" {
		return "", fmt.Errorf("AEON_DATABASE_PASSWORD_FILE is empty")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return "", fmt.Errorf("AEON_DATABASE_URL must be a postgres:// URL with a user")
	}
	u.User = url.UserPassword(u.User.Username(), pw)
	return u.String(), nil
}
