// SPDX-License-Identifier: AGPL-3.0-only
package attachments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VerifyIssue struct {
	SHA256  string `json:"sha256"`
	Variant string `json:"variant"`
	Problem string `json:"problem"`
}
type VerifyReport struct {
	Checked int           `json:"checked"`
	Issues  []VerifyIssue `json:"issues"`
}

// Verify checks every blob referenced by a live or soft-deleted attachment.
// Soft-deleted rows remain referenced because undo can restore them.
func Verify(ctx context.Context, pool *pgxpool.Pool, store Store, tenantID string) (VerifyReport, error) {
	report := VerifyReport{Issues: []VerifyIssue{}}
	refs, err := references(ctx, pool, tenantID)
	if err != nil {
		return report, err
	}
	for hash, isImage := range refs {
		report.Checked++
		expected := map[string]string{"original": hash}
		if isImage {
			original, err := store.Open(tenantID, hash, "original")
			if err == nil {
				decoded, _, decodeErr := image.Decode(original)
				_ = original.Close()
				if decodeErr == nil {
					for _, spec := range []struct {
						name string
						max  int
					}{{"thumb", 320}, {"preview", 1600}} {
						var body bytes.Buffer
						if png.Encode(&body, resize(decoded, spec.max)) == nil {
							sum := sha256.Sum256(body.Bytes())
							expected[spec.name] = hex.EncodeToString(sum[:])
						}
					}
				} else {
					report.Issues = append(report.Issues, VerifyIssue{hash, "original", "invalid image"})
				}
			}
		}
		for _, variant := range []string{"original", "preview", "thumb"} {
			if variant != "original" && !isImage {
				continue
			}
			f, err := store.Open(tenantID, hash, variant)
			if err != nil {
				problem := "unreadable"
				if errors.Is(err, os.ErrNotExist) {
					problem = "missing"
				}
				report.Issues = append(report.Issues, VerifyIssue{hash, variant, problem})
				continue
			}
			h := sha256.New()
			_, copyErr := io.Copy(h, f)
			closeErr := f.Close()
			if copyErr != nil || closeErr != nil {
				report.Issues = append(report.Issues, VerifyIssue{hash, variant, "unreadable"})
				continue
			}
			if want := expected[variant]; want != "" && hex.EncodeToString(h.Sum(nil)) != want {
				report.Issues = append(report.Issues, VerifyIssue{hash, variant, "corrupt"})
			}
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
	}
	return report, nil
}
func references(ctx context.Context, pool *pgxpool.Pool, tenantID string) (map[string]bool, error) {
	if !validTenant(tenantID) {
		return nil, errors.New("invalid tenant")
	}
	refs := map[string]bool{}
	err := db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT sha256,content_type FROM attachments WHERE tenant_id=$1`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var hash, ct string
			if err := rows.Scan(&hash, &ct); err != nil {
				return err
			}
			refs[hash] = refs[hash] || strings.HasPrefix(ct, "image/")
		}
		return rows.Err()
	})
	return refs, err
}

type GCReport struct {
	Candidates int   `json:"candidates"`
	Removed    int   `json:"removed"`
	Bytes      int64 `json:"bytes"`
}

// GC examines only the selected tenant. apply=false is the dry-run default.
// A seven-day minimum age protects in-flight uploads and failed transactions.
func GC(ctx context.Context, pool *pgxpool.Pool, store Store, tenantID string, apply bool) (GCReport, error) {
	report := GCReport{}
	refs, err := references(ctx, pool, tenantID)
	if err != nil {
		return report, err
	}
	base := filepath.Join(store.root(), tenantID)
	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		hash := strings.TrimSuffix(strings.TrimSuffix(name, ".thumb.png"), ".preview.png")
		spool := filepath.Dir(path) == filepath.Join(base, "incoming") && strings.HasPrefix(name, "upload-")
		if !spool && (!validHash(hash) || refs[hash]) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) < 7*24*time.Hour {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		report.Candidates++
		report.Bytes += info.Size()
		if apply {
			if !spool {
				var referenced bool
				if err := db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM attachments WHERE tenant_id=$1 AND sha256=$2)`, tenantID, hash).Scan(&referenced)
				}); err != nil {
					return err
				}
				if referenced {
					report.Candidates--
					report.Bytes -= info.Size()
					return nil
				}
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			report.Removed++
		}
		return nil
	})
	return report, err
}
