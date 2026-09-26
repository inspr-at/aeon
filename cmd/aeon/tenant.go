// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/config"
	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/tenantbootstrap"
)

// tenantCommand runs the operator-only tenant bootstrap commands:
//
//	paimos tenant create --slug SLUG --name NAME
//	paimos tenant principal bind-oidc --tenant SLUG --issuer URL --subject SUBJECT --name NAME --role admin|member|customer
func tenantCommand(args []string, stdout io.Writer) error {
	const usage = "usage: paimos tenant create --slug SLUG --name NAME | paimos tenant principal bind-oidc --tenant SLUG --issuer URL --subject SUBJECT --name NAME --role admin|member|customer"
	switch {
	case len(args) > 0 && args[0] == "create":
		flags := flag.NewFlagSet("paimos tenant create", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		slug := flags.String("slug", "", "tenant slug")
		name := flags.String("name", "", "tenant name")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *slug == "" || *name == "" {
			return errors.New(usage)
		}
		return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
			id, err := tenantbootstrap.Create(ctx, pool, *slug, *name)
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(map[string]string{"tenant_id": id, "slug": *slug})
		})
	case len(args) > 1 && args[0] == "principal" && args[1] == "bind-oidc":
		flags := flag.NewFlagSet("paimos tenant principal bind-oidc", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		tenant := flags.String("tenant", "", "tenant slug")
		issuer := flags.String("issuer", "", "OIDC issuer URL")
		subject := flags.String("subject", "", "OIDC subject")
		name := flags.String("name", "", "display name")
		role := flags.String("role", "", "admin, member or customer")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *tenant == "" || *issuer == "" || *subject == "" || *name == "" || *role == "" {
			return errors.New(usage)
		}
		return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
			id, err := tenantbootstrap.BindOIDC(ctx, pool, *tenant, *issuer, *subject, *name, *role)
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(map[string]string{"principal_id": id, "tenant": *tenant})
		})
	}
	return errors.New(usage)
}

func withPool(run func(context.Context, *pgxpool.Pool) error) error {
	ctx := context.Background()
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()
	return run(ctx, pool)
}
