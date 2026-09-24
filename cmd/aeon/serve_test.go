// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/config"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/web"
)

func TestLoggerJSONInProd(t *testing.T) {
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "listening", 0)
	var buf bytes.Buffer
	if err := loggerHandler("prod", &buf).Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) || !bytes.Contains(buf.Bytes(), []byte(`"msg":"listening"`)) {
		t.Fatalf("prod log is not json: %s", buf.String())
	}
	buf.Reset()
	if err := loggerHandler("dev", &buf).Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("dev log looks like json: %s", buf.String())
	}
}

func TestResolveWeb(t *testing.T) {
	fsys, err := resolveWeb(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if embedded, ok := web.Static(); ok {
		if fsys == nil {
			t.Fatal("expected embedded assets")
		}
		if _, err := fs.ReadFile(embedded, "index.html"); err != nil {
			t.Fatal(err)
		}
	} else if fsys != nil {
		t.Fatal("expected no embedded assets without the webembed tag")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys, err = resolveWeb(config.Config{WebDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil || string(b) != "hi" {
		t.Fatalf("read %q err %v", b, err)
	}
	if _, err := resolveWeb(config.Config{WebDir: filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("expected missing web dir to fail")
	}
}

func TestServeShutdownAndBootstrap(t *testing.T) {
	// Auth reads AEON_ENV itself and no longer treats an unset value as dev.
	t.Setenv("AEON_ENV", "dev")
	fresh := dbtest.Open(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DatabaseURL:         fresh.URL,
		Env:                 "dev",
		PublicURL:           "http://127.0.0.1",
		BootstrapTenantSlug: "p02-boot",
		BootstrapTenantName: "Boot",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- serveListener(ctx, cfg, ln)
	}()

	base := "http://" + ln.Addr().String()
	var resp *http.Response
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err = http.Get(base + "/api/health")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"db":"ok"`)) {
		t.Fatalf("health %d %s", resp.StatusCode, body)
	}

	var name string
	if err := fresh.Admin.QueryRow(context.Background(), `SELECT name FROM tenants WHERE slug = 'p02-boot'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Boot" {
		t.Fatalf("bootstrap name %s", name)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("shutdown timed out")
	}
}
