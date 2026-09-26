// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const accessUsage = "usage: aeon access bind --tenant SLUG --principal NAME|ID (--project KEY --role ROLEKEY | --workspace-role ROLEKEY) | aeon access unbind --tenant SLUG --principal NAME|ID (--project KEY | --workspace-role)"

// accessCommand is host-only and uses the operator database connection.
func accessCommand(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "bind" && args[0] != "unbind" {
		return errors.New(accessUsage)
	}
	flags := flag.NewFlagSet("aeon access "+args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tenantSlug := flags.String("tenant", "", "tenant slug")
	principal := flags.String("principal", "", "principal name or UUID")
	project := flags.String("project", "", "project key")
	role := flags.String("role", "", "role key")
	var workspaceRole string
	var workspaceUnbind bool
	if args[0] == "bind" {
		flags.StringVar(&workspaceRole, "workspace-role", "", "workspace role key")
	} else {
		flags.BoolVar(&workspaceUnbind, "workspace-role", false, "remove workspace role")
	}
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *tenantSlug == "" || *principal == "" ||
		args[0] == "bind" && ((*project != "" && (*role == "" || workspaceRole != "")) || (*project == "" && (*role != "" || workspaceRole == ""))) ||
		args[0] == "unbind" && (*role != "" || (*project == "") == !workspaceUnbind) {
		return errors.New(accessUsage)
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		tenantID, err := operatorTenantID(ctx, pool, *tenantSlug)
		if err != nil {
			return err
		}
		if args[0] == "bind" {
			if workspaceRole != "" {
				if err := authz.OperatorBindWorkspaceRole(ctx, pool, tenantID, *principal, workspaceRole); err != nil {
					return err
				}
				return json.NewEncoder(stdout).Encode(map[string]string{"principal": *principal, "workspace_role": workspaceRole, "status": "bound"})
			}
			bindings, err := authz.OperatorBindProjects(ctx, pool, tenantID, *principal, []authz.ProjectRolePair{{Project: *project, Role: *role}})
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(bindings[0])
		}
		if workspaceUnbind {
			if err := authz.OperatorUnbindWorkspaceRole(ctx, pool, tenantID, *principal); err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(map[string]string{"principal": *principal, "workspace_role": "", "status": "unbound"})
		}
		if err := authz.OperatorUnbindProject(ctx, pool, tenantID, *principal, *project); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(map[string]string{"principal": *principal, "project": *project, "status": "unbound"})
	})
}

func operatorTenantID(ctx context.Context, pool *pgxpool.Pool, slug string) (string, error) {
	var id string
	err := db.InTenant(db.NoProjects(ctx, "operator tenant lookup"), pool, "00000000-0000-0000-0000-000000000000", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug=$1`, slug).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("tenant %q not found", slug)
	}
	return id, err
}

type projectFlags []string

func (f *projectFlags) String() string { return strings.Join(*f, ",") }
func (f *projectFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func agentProjectRoles(projects, roles projectFlags) ([]authz.ProjectRolePair, error) {
	if len(projects) == 0 && len(roles) == 0 {
		return nil, nil
	}
	if len(roles) == 0 {
		var pairs []authz.ProjectRolePair
		for _, value := range projects {
			for _, item := range strings.Split(value, ",") {
				project, role, ok := strings.Cut(item, "=")
				if !ok || strings.TrimSpace(project) == "" || strings.TrimSpace(role) == "" {
					return nil, errors.New("use --project KEY=ROLE[,KEY=ROLE] or paired --project KEY --project-role ROLE")
				}
				pairs = append(pairs, authz.ProjectRolePair{Project: strings.TrimSpace(project), Role: strings.TrimSpace(role)})
			}
		}
		return pairs, nil
	}
	if len(projects) != len(roles) {
		return nil, errors.New("each --project needs one --project-role")
	}
	pairs := make([]authz.ProjectRolePair, len(projects))
	for i := range projects {
		if projects[i] == "" || roles[i] == "" || strings.ContainsAny(projects[i], "=,") {
			return nil, errors.New("invalid project/role pair")
		}
		pairs[i] = authz.ProjectRolePair{Project: projects[i], Role: roles[i]}
	}
	return pairs, nil
}
