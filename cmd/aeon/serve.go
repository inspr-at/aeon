// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/inspr-at/aeon/internal/agentaccounts"
	"github.com/inspr-at/aeon/internal/agentruns"
	"github.com/inspr-at/aeon/internal/approvals"
	"github.com/inspr-at/aeon/internal/embedding"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/imports"
	"github.com/inspr-at/aeon/internal/inbox"
	"github.com/inspr-at/aeon/internal/intake"
	"github.com/inspr-at/aeon/internal/journey"
	"github.com/inspr-at/aeon/internal/modelregistry"
	"github.com/inspr-at/aeon/internal/nodes"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/relations"
	"github.com/inspr-at/aeon/internal/releases"
	"github.com/inspr-at/aeon/internal/requirements"
	"github.com/inspr-at/aeon/internal/search"
	"github.com/inspr-at/aeon/internal/stagehandoff"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/inspr-at/aeon/internal/views"
	"github.com/inspr-at/aeon/internal/workorders"
	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/auth"

	"github.com/inspr-at/aeon/internal/config"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/web"
)

func serve() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return serveListener(ctx, cfg, ln)
}

func serveListener(ctx context.Context, cfg config.Config, ln net.Listener) error {
	setupLogger(cfg.Env)

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		_ = ln.Close()
		return err
	}
	defer pool.Close()

	if err := db.EnsureTenant(ctx, pool, cfg.BootstrapTenantSlug, cfg.BootstrapTenantName); err != nil {
		_ = ln.Close()
		return err
	}

	webFS, err := resolveWeb(cfg)
	if err != nil {
		_ = ln.Close()
		return err
	}

	authCfg, err := auth.FromEnv()
	if err != nil {
		_ = ln.Close()
		return err
	}
	authMod, err := auth.New(authCfg, pool)
	if err != nil {
		_ = ln.Close()
		return err
	}
	// R1: embeddings are optional; without AEON_EMBEDDING_URL search is lexical only.
	pluginRegistry, err := plugins.Builtin()
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("plugins: %w", err)
	}
	embedProvider, err := embedding.FromEnv()
	if err != nil {
		_ = ln.Close()
		return err
	}
	if embedProvider != nil {
		go embedding.NewWorker(pool, embedProvider, embedding.Options{}).Run(ctx)
	}
	// R2: webhook wake for inbox deliveries.
	go inbox.NewWorker(pool, inbox.WorkerOptions{}).Run(ctx)
	api := &httpapi.Server{
		Pool: pool,
		Web:  webFS,
		Modules: []httpapi.Module{
			authMod,
			nodes.New(pool, nodes.SQLWriter{}),
			relations.New(pool),
			events.New(pool),
			search.New(pool, embedProvider),
			views.New(pool),
			imports.New(pool),
			// R2: agents
			inbox.New(pool),
			workorders.New(pool),
			agentruns.New(pool, settleUsage),
			approvals.New(pool),
			modelregistry.New(pool),
			agentaccounts.New(pool),
			// R3: journey
			journey.New(pool),
			requirements.New(pool),
			releases.New(pool),
			intake.New(pool),
			plugins.NewWithRegistry(pool, pluginRegistry),
			// No LaunchChecks provider yet: stage launch admission fails closed.
			stagehandoff.New(pool, pluginRegistry),
		},
		Middleware: []func(http.Handler) http.Handler{authMod.Middleware},
	}
	srv := &http.Server{
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- err
	}()

	slog.Info("aeon listening", "addr", ln.Addr().String(), "env", cfg.Env, "public_url", cfg.PublicURL)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			return err
		}
		return <-errCh
	}
}

func setupLogger(env string) {
	slog.SetDefault(slog.New(loggerHandler(env, os.Stdout)))
}

func loggerHandler(env string, w io.Writer) slog.Handler {
	if env == "prod" {
		return slog.NewJSONHandler(w, nil)
	}
	return slog.NewTextHandler(w, nil)
}

func resolveWeb(cfg config.Config) (fs.FS, error) {
	if cfg.WebDir != "" {
		info, err := os.Stat(cfg.WebDir)
		if err != nil {
			return nil, fmt.Errorf("AEON_WEB_DIR: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("AEON_WEB_DIR %s is not a directory", cfg.WebDir)
		}
		return os.DirFS(cfg.WebDir), nil
	}
	if fsys, ok := web.Static(); ok {
		return fsys, nil
	}
	return nil, nil
}

// settleUsage lets finished runs settle their account allowance projections (R2).
func settleUsage(ctx context.Context, tx pgx.Tx, p tenant.Principal, run agentruns.Run, _ agentruns.Telemetry) error {
	return agentaccounts.Settle(ctx, tx, p, run.ID)
}
