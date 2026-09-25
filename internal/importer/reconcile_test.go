// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/attachments"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
)

type cancelOnWrite struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelOnWrite) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.cancel()
	return n, err
}

func TestReconcileReportsClassic404Findings(t *testing.T) {
	ctx := context.Background()
	source, closeSource := fakeClassic(t)
	defer closeSource()
	d := dbtest.Open(t)
	tenantID, err := tenantbootstrap.Create(ctx, d.App, "reconcileskips", "Reconcile skips")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Importer{Source: source, Writer: PostgresWriter{Pool: d.App}}).RunDelta(ctx, "reconcileskips", ""); err != nil {
		t.Fatal(err)
	}
	snap, err := source.Read(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var actorID string
	if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE tenant_id=$1 AND name='Classic Paimos importer'`, tenantID).Scan(&actorID)
	}); err != nil {
		t.Fatal(err)
	}
	store := attachments.Store{FilesDir: t.TempDir()}
	if _, err := ImportAttachments(ctx, d.App, store, source, snap, tenantID, actorID); err != nil {
		t.Fatal(err)
	}
	transport := source.client.Transport
	source.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Errorf("classic write: %s", r.Method)
		}
		if r.URL.Path == "/api/issues/10/relations" || r.URL.Path == "/api/attachments/50" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
		}
		return transport.RoundTrip(r)
	})
	var progress bytes.Buffer
	report, err := ReconcileWithOptions(ctx, source, d.App, store, "reconcileskips", "", ReconcileOptions{Progress: &progress, Every: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Partial || report.SkippedCount != 2 || report.Progress.Skipped != 2 {
		t.Fatalf("report: %+v", report)
	}
	if report.Skipped[0] != (ReconcileFinding{Kind: "attachment", ClassicID: "50", Reason: "HTTP 404 GET /attachments/50"}) ||
		report.Skipped[1] != (ReconcileFinding{Kind: "issue", ClassicID: "10", Reason: "HTTP 404 GET /issues/10/relations"}) {
		t.Fatalf("findings: %+v", report.Skipped)
	}
	for _, p := range report.Projects {
		if p.ClassicID == 3 {
			if len(p.Categories["tickets"].Extra) != 0 || len(p.Categories["attachments"].Extra) != 0 {
				t.Fatalf("skipped items became extra: %+v", p.Categories)
			}
		}
	}
	if !strings.Contains(progress.String(), "project=PAI") || !strings.Contains(progress.String(), "skipped=2") {
		t.Fatal(progress.String())
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"skipped_count":2`) {
		t.Fatalf("JSON %s: %v", encoded, err)
	}
	source.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Errorf("classic write: %s", r.Method)
		}
		if r.URL.Path == "/api/projects/3/issues" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
		}
		return transport.RoundTrip(r)
	})
	report, err = ReconcileWithOptions(ctx, source, d.App, store, "reconcileskips", "PAI", ReconcileOptions{})
	if err != nil || report.SkippedCount != 1 || report.Skipped[0].Kind != "project" || report.Skipped[0].ClassicID != "3" {
		t.Fatalf("skipped project: %+v %v", report, err)
	}
	source.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
	})
	var partial bytes.Buffer
	if _, err := ReconcileWithOptions(ctx, source, d.App, store, "reconcileskips", "", ReconcileOptions{Partial: &partial}); err == nil || !strings.Contains(err.Error(), "HTTP 401") || partial.Len() != 0 {
		t.Fatalf("auth failure must abort without partial report: %v %q", err, partial.String())
	}
}

func TestReconcileWritesPartialOnInterrupt(t *testing.T) {
	source, closeSource := fakeClassic(t)
	defer closeSource()
	d := dbtest.Open(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	progress := &cancelOnWrite{cancel: cancel}
	var partial bytes.Buffer
	_, err := ReconcileWithOptions(ctx, source, d.App, attachments.Store{FilesDir: t.TempDir()}, "unused", "", ReconcileOptions{Progress: progress, Partial: &partial, Every: 1})
	if err == nil {
		t.Fatal("expected interruption")
	}
	var report ReconcileReport
	if err := json.Unmarshal(partial.Bytes(), &report); err != nil {
		t.Fatalf("partial JSON: %v (%q)", err, partial.String())
	}
	if !report.Partial || report.Progress.Projects < 1 || len(report.Projects) != 0 {
		t.Fatalf("partial report: %+v", report)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReconcileClassicBytesAndDeltaConflict(t *testing.T) {
	ctx := context.Background()
	source, closeSource := fakeClassic(t)
	defer closeSource()
	d := dbtest.Open(t)
	tenantID, err := tenantbootstrap.Create(ctx, d.App, "reconcile", "Reconcile")
	if err != nil {
		t.Fatal(err)
	}
	job := Importer{Source: source, Writer: PostgresWriter{Pool: d.App}}
	store := attachments.Store{FilesDir: t.TempDir()}
	first, err := job.RunDelta(ctx, "reconcile", "")
	if err != nil || first.Created != 5 {
		t.Fatalf("first delta: %+v %v", first, err)
	}
	second, err := job.RunDelta(ctx, "reconcile", "")
	if err != nil || second.Created != 0 || second.Updated != 0 || len(second.Conflicts) != 0 {
		t.Fatalf("idempotent delta: %+v %v", second, err)
	}
	report, err := Reconcile(ctx, source, d.App, store, "reconcile", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Projects[1].Categories["attachments"].Missing; len(got) != 1 || got[0].ClassicID != "50" {
		t.Fatalf("missing attachment bytes: %+v", got)
	}
	var actorID string
	if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE tenant_id=$1 AND name='Classic Paimos importer'`, tenantID).Scan(&actorID)
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := source.Read(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := ImportAttachments(ctx, d.App, store, source, snap, tenantID, actorID)
	if err != nil || created != 1 {
		t.Fatalf("attachment import: %d %v", created, err)
	}
	report, err = Reconcile(ctx, source, d.App, store, "reconcile", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range report.Projects {
		for category, result := range project.Categories {
			if len(result.Missing)+len(result.Extra)+len(result.Changed) != 0 || result.SourceChecksum != result.AeonChecksum {
				t.Errorf("%s %s: %+v", project.Key, category, result)
			}
		}
	}
	if !strings.Contains(report.Summary, "0 missing, 0 extra, 0 changed") {
		t.Fatal(report.Summary)
	}
	fileHash := sha256.Sum256([]byte("data"))
	file, err := store.Open(tenantID, hex.EncodeToString(fileHash[:]), "original")
	if err != nil {
		t.Fatal(err)
	}
	filePath := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("bad!"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err = Reconcile(ctx, source, d.App, store, "reconcile", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Projects[1].Categories["attachments"].Changed; len(got) != 1 || got[0].ClassicID != "50" {
		t.Fatalf("corrupt Aeon bytes not detected: %+v", got)
	}
	var nodeID string
	if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
		var before, after []byte
		if err := tx.QueryRow(ctx, `SELECT id::text,to_jsonb(n) FROM nodes n WHERE tenant_id=$1 AND key='PAI-11'`, tenantID).Scan(&nodeID, &before); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET title='Aeon edit' WHERE tenant_id=$1 AND id=$2`, tenantID, nodeID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT to_jsonb(n) FROM nodes n WHERE tenant_id=$1 AND id=$2`, tenantID, nodeID).Scan(&after); err != nil {
			return err
		}
		_, err := events.Append(ctx, tx, tenant.Principal{TenantID: tenantID, ID: actorID}, events.Change{Type: "node.updated", NodeID: &nodeID, Before: json.RawMessage(before), After: json.RawMessage(after)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for n := range snap.Projects[0].Issues {
		if id, _ := intField(snap.Projects[0].Issues[n], "id"); id == 11 {
			snap.Projects[0].Issues[n]["title"] = "Classic edit"
		}
	}
	snap.Details[11] = Details{Relations: snap.Details[11].Relations, Comments: append(snap.Details[11].Comments, Record{"id": 31, "issue_id": 11, "body": "new classic comment"}), History: snap.Details[11].History, Attachments: snap.Details[11].Attachments}
	delta, err := (PostgresWriter{Pool: d.App}).Write(ctx, snap, "reconcile")
	if err != nil || len(delta.Conflicts) != 1 || delta.Conflicts[0].ClassicID != 11 {
		t.Fatalf("delta conflict: %+v %v", delta, err)
	}
	if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
		var title string
		var comments int
		if err := tx.QueryRow(ctx, `SELECT title FROM nodes WHERE tenant_id=$1 AND id=$2`, tenantID, nodeID).Scan(&title); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM events WHERE tenant_id=$1 AND type='import.comment'`, tenantID).Scan(&comments); err != nil {
			return err
		}
		if title != "Aeon edit" || comments != 2 {
			t.Errorf("title %q, comments %d", title, comments)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentDeltaRefreshesChangedClassicBytes(t *testing.T) {
	ctx := context.Background()
	source, closeSource := fakeClassic(t)
	defer closeSource()
	d := dbtest.Open(t)
	tenantID, err := tenantbootstrap.Create(ctx, d.App, "attachmentdelta", "Attachment delta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Importer{Source: source, Writer: PostgresWriter{Pool: d.App}}).RunDelta(ctx, "attachmentdelta", ""); err != nil {
		t.Fatal(err)
	}
	snap, err := source.Read(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var actorID string
	if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE tenant_id=$1 AND name='Classic Paimos importer'`, tenantID).Scan(&actorID)
	}); err != nil {
		t.Fatal(err)
	}
	store := attachments.Store{FilesDir: t.TempDir()}
	if n, err := ImportAttachments(ctx, d.App, store, source, snap, tenantID, actorID); err != nil || n != 1 {
		t.Fatalf("first attachment import: %d %v", n, err)
	}
	transport := source.client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	source.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/attachments/50" {
			if r.Method != http.MethodGet {
				t.Errorf("classic write: %s", r.Method)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("new bytes")), Header: http.Header{}, Request: r}, nil
		}
		return transport.RoundTrip(r)
	})
	delta, err := ImportAttachmentDelta(ctx, d.App, store, source, snap, tenantID, actorID)
	if err != nil || delta.Updated != 1 || delta.Created != 0 || len(delta.Conflicts) != 0 {
		t.Fatalf("attachment delta: %+v %v", delta, err)
	}
	again, err := ImportAttachmentDelta(ctx, d.App, store, source, snap, tenantID, actorID)
	if err != nil || again.Updated != 0 || again.Created != 0 {
		t.Fatalf("attachment replay: %+v %v", again, err)
	}
	report, err := Reconcile(ctx, source, d.App, store, "attachmentdelta", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Projects[1].Categories["attachments"]; len(got.Changed)+len(got.Missing)+len(got.Extra) != 0 {
		t.Fatalf("attachment delta did not reconcile: %+v", got)
	}
}
