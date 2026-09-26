// SPDX-License-Identifier: AGPL-3.0-only

package journey

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/operatoractor"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
)

const operatorUsage = "usage: aeon journey mark-disposable --tenant SLUG --project KEY | aeon journey seed --tenant SLUG --project KEY --to-stage build|candidate|deploy"

// OperatorResult is the value-free result of a host-only journey command.
type OperatorResult struct {
	ProjectKey    string  `json:"project_key"`
	Disposable    bool    `json:"disposable"`
	Stage         string  `json:"stage"`
	ReleaseID     *string `json:"release_id"`
	TargetReached bool    `json:"target_reached"`
	PendingAction string  `json:"pending_action,omitempty"`
	Already       bool    `json:"already"`
}

// RunOperator implements the host CLI's `aeon journey` subcommands. The
// coordinator wires it into cmd/aeon; no HTTP route exposes these operations.
// Both subcommands require AEON_ENV=dev, an explicit tenant and exact node key.
func RunOperator(ctx context.Context, pool *pgxpool.Pool, args []string, stdout io.Writer) error {
	if os.Getenv("AEON_ENV") != "dev" {
		return errors.New("journey operator commands require AEON_ENV=dev")
	}
	if len(args) == 0 || args[0] != "mark-disposable" && args[0] != "seed" {
		return errors.New(operatorUsage)
	}
	fs := flag.NewFlagSet("aeon journey "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	slug := fs.String("tenant", "", "tenant slug")
	project := fs.String("project", "", "project node key")
	var toStage *string
	if args[0] == "seed" {
		toStage = fs.String("to-stage", "", "target stage")
	}
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *slug == "" || *project == "" ||
		args[0] == "seed" && (*toStage != "build" && *toStage != "candidate" && *toStage != "deploy") {
		return errors.New(operatorUsage)
	}
	var out OperatorResult
	var err error
	if args[0] == "mark-disposable" {
		out, err = MarkDisposable(ctx, pool, *slug, *project)
	} else {
		out, err = SeedDisposable(ctx, pool, *slug, *project, *toStage)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(out)
}

func operatorContext(ctx context.Context) (context.Context, error) {
	if os.Getenv("AEON_ENV") != "dev" {
		return nil, errors.New("journey operator commands require AEON_ENV=dev")
	}
	return db.AllProjects(ctx, "disposable journey operator command"), nil
}

func projectByKey(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("project node key is required")
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT n.id::text FROM nodes n
		JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
		WHERE n.key=$1 AND n.deleted_at IS NULL AND k.slug='project'`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("project %q not found", key)
	}
	return id, err
}

// MarkDisposable records an explicit, irreversible development marker. A live
// or previously released project cannot be marked. A replay makes no changes.
func MarkDisposable(ctx context.Context, pool *pgxpool.Pool, slug, key string) (OperatorResult, error) {
	ctx, err := operatorContext(ctx)
	if err != nil {
		return OperatorResult{}, err
	}
	if pool == nil {
		return OperatorResult{}, errors.New("database pool is required")
	}
	var out OperatorResult
	err = db.InTransaction(ctx, pool, func(ctx context.Context) error {
		tid, err := tenantbootstrap.ResolveSlug(ctx, pool, slug)
		if err != nil {
			return err
		}
		return db.InTenant(ctx, pool, tid, func(tx pgx.Tx) error {
			id, err := projectByKey(ctx, tx, key)
			if err != nil {
				return err
			}
			var already bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM journey_disposable_projects WHERE project_node_id=$1::uuid)`, id).Scan(&already); err != nil {
				return err
			}
			if !already {
				var live bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM journey_releases WHERE project_node_id=$1::uuid AND state IN ('released','superseded','deploying','access'))`, id).Scan(&live); err != nil {
					return err
				}
				if live {
					return errors.New("cannot mark a deployed or released project disposable")
				}
				actorID, err := operatoractor.Ensure(ctx, tx, tid)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO journey_disposable_projects(tenant_id,project_node_id,marked_by_principal_id) VALUES($1::uuid,$2::uuid,$3::uuid)`, tid, id, actorID); err != nil {
					return err
				}
				p := tenant.Principal{ID: actorID, TenantID: tid, Kind: tenant.Agent}
				if _, err := writeEvent(ctx, tx, p, id, "journey.disposable_marked", nil, map[string]any{"project_node_id": id, "disposable": true}); err != nil {
					return err
				}
			}
			out = OperatorResult{ProjectKey: key, Disposable: true, Already: already}
			return nil
		})
	})
	return out, err
}

// SeedDisposable advances existing, real project/release work through journey
// actions in one transaction. It may waive the build gate on an explicitly
// disposable project, but never fabricates a requirement agreement, completed
// ticket, candidate review, deployment decision or Pharos evidence. Missing
// work prerequisites roll back all transitions. A pending human gate commits
// valid preparation and returns the next action; rerun after the UI decision.
func SeedDisposable(ctx context.Context, pool *pgxpool.Pool, slug, key, target string) (OperatorResult, error) {
	ctx, err := operatorContext(ctx)
	if err != nil {
		return OperatorResult{}, err
	}
	if pool == nil || target != "build" && target != "candidate" && target != "deploy" {
		return OperatorResult{}, errors.New("database pool and build|candidate|deploy target are required")
	}
	var out OperatorResult
	err = db.InTransaction(ctx, pool, func(ctx context.Context) error {
		tid, err := tenantbootstrap.ResolveSlug(ctx, pool, slug)
		if err != nil {
			return err
		}
		var id, actorID string
		if err := db.InTenant(ctx, pool, tid, func(tx pgx.Tx) error {
			var err error
			id, err = projectByKey(ctx, tx, key)
			if err != nil {
				return err
			}
			var disposable bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM journey_disposable_projects WHERE project_node_id=$1::uuid)`, id).Scan(&disposable); err != nil {
				return err
			}
			if !disposable {
				return errors.New("project is not disposable")
			}
			actorID, err = operatoractor.Ensure(ctx, tx, tid)
			return err
		}); err != nil {
			return err
		}
		p := tenant.Principal{ID: actorID, TenantID: tid, Kind: tenant.Agent}
		m := &Module{pool: pool, inTenant: db.InTenant}
		changed := false
		for step := 0; step <= 3; step++ {
			view, err := m.read(ctx, p, id)
			if err != nil {
				return err
			}
			state, err := releaseState(ctx, pool, tid, view.CurrentReleaseID)
			if err != nil {
				return err
			}
			reached := targetReached(state, view.Stage, target)
			if reached && target == "deploy" {
				reached, err = deployGatesLive(ctx, pool, tid, id)
				if err != nil {
					return err
				}
			}
			if reached {
				out = OperatorResult{ProjectKey: key, Disposable: true, Stage: view.Stage, ReleaseID: view.CurrentReleaseID, TargetReached: true, Already: !changed}
				return nil
			}
			action := seedNextAction(view, target)
			if action == "" {
				if view.NextAction.Key == "approve_candidate" || view.NextAction.Key == "approve_deploy" {
					out = OperatorResult{ProjectKey: key, Disposable: true, Stage: view.Stage, ReleaseID: view.CurrentReleaseID, PendingAction: view.NextAction.Key}
					return nil
				}
				return fmt.Errorf("cannot seed to %s: %s (%s)", target, view.NextAction.Key, view.NextAction.Reason)
			}
			if step == 3 {
				return errors.New("journey seed exceeded three actions")
			}
			in := actionWrite{Action: action, ExpectedRevision: view.Revision, IdempotencyKey: fmt.Sprintf("operator-seed:%s:%d", action, view.Revision), ReleaseID: view.CurrentReleaseID}
			if _, err := m.actWithMode(ctx, p, id, in, true); err != nil {
				return fmt.Errorf("seed %s: %w", action, err)
			}
			changed = true
		}
		return errors.New("journey seed exceeded three actions")
	})
	return out, err
}

func releaseState(ctx context.Context, pool *pgxpool.Pool, tid string, id *string) (string, error) {
	if id == nil {
		return "", nil
	}
	var state string
	err := db.InTenant(ctx, pool, tid, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM journey_releases WHERE release_node_id=$1::uuid`, *id).Scan(&state)
	})
	return state, err
}

func deployGatesLive(ctx context.Context, pool *pgxpool.Pool, tid, projectID string) (bool, error) {
	var live bool
	err := db.InTenant(ctx, pool, tid, func(tx pgx.Tx) error {
		f, err := loadFacts(ctx, tx, projectID, false)
		if err != nil {
			return err
		}
		live = f.CandidateGateID != "" && f.DeployGateID != "" &&
			f.GateLiveByID[f.CandidateGateID] && f.GateLiveByID[f.DeployGateID]
		return nil
	})
	return live, err
}

func targetReached(state, stage, target string) bool {
	switch target {
	case "build":
		return stage == "build" || state == "building" || state == "candidate" || state == "deploying" || state == "access" || state == "released" || state == "superseded"
	case "candidate":
		return state == "candidate" || state == "deploying" || state == "access" || state == "released" || state == "superseded"
	case "deploy":
		return state == "deploying" || state == "access" || state == "released" || state == "superseded"
	}
	return false
}

func seedNextAction(v Journey, target string) string {
	switch v.NextAction.Key {
	case "open_first_release":
		return "open_first_release"
	case "start_build":
		return "start_build"
	case "mark_candidate":
		if target != "build" {
			return "mark_candidate"
		}
	}
	return ""
}
