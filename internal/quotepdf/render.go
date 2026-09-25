// SPDX-License-Identifier: AGPL-3.0-only

package quotepdf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/attachments"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const RendererVersion = "aeon-quote-document-1"

type Stamp struct {
	Name    string `json:"name"`
	Company string `json:"company,omitempty"`
	At      string `json:"at"`
	Digest  string `json:"digest"`
}

type Payload struct {
	Document      json.RawMessage         `json:"document"`
	OfferNo       string                  `json:"offer_no"`
	PublicURL     string                  `json:"public_url,omitempty"`
	Accepted      *Stamp                  `json:"accepted,omitempty"`
	ProfileAssets map[string]ProfileAsset `json:"-"`
}

type ProfileAsset struct {
	ContentType string
	Bytes       []byte
}

// LoadProfileAssets resolves only IDs frozen into the issued document, using
// tenant RLS. Render serves them on its private loopback origin under font-src
// and img-src 'self'; no production API credentials enter Chromium.
func LoadProfileAssets(ctx context.Context, pool *pgxpool.Pool, store attachments.Store, tenantID string, document json.RawMessage) (map[string]ProfileAsset, error) {
	var wrapped struct {
		Profile *struct {
			Definition struct {
				Fonts []struct {
					AssetID string `json:"asset_id"`
				} `json:"fonts"`
				Footer struct {
					AssetID string `json:"asset_id"`
				} `json:"footer"`
			} `json:"definition"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(document, &wrapped); err != nil {
		return nil, err
	}
	assets := map[string]ProfileAsset{}
	if wrapped.Profile == nil {
		return assets, nil
	}
	ids := map[string]bool{}
	for _, f := range wrapped.Profile.Definition.Fonts {
		ids[f.AssetID] = true
	}
	if id := wrapped.Profile.Definition.Footer.AssetID; id != "" {
		ids[id] = true
	}
	if len(ids) > 13 {
		return nil, errors.New("too many profile assets")
	}
	var size int64
	err := db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		for id := range ids {
			var hash, kind string
			var length int64
			if err := tx.QueryRow(ctx, `SELECT sha256,content_type,size FROM quote_document_profile_assets WHERE id=$1::uuid`, id).Scan(&hash, &kind, &length); err != nil {
				return err
			}
			size += length
			if size > 20<<20 {
				return errors.New("profile assets exceed PDF limit")
			}
			f, err := store.Open(tenantID, hash, "original")
			if err != nil {
				return err
			}
			b, readErr := io.ReadAll(io.LimitReader(f, length+1))
			closeErr := f.Close()
			if readErr != nil {
				return readErr
			}
			if closeErr != nil {
				return closeErr
			}
			if int64(len(b)) != length {
				return errors.New("profile asset length mismatch")
			}
			assets[id] = ProfileAsset{ContentType: kind, Bytes: b}
		}
		return nil
	})
	return assets, err
}

func Available() bool {
	for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return true
		}
	}
	return false
}

// Render serves a private, bounded print entry to a fresh Chromium process.
// No network resource from the document is allowed by the page CSP.
func Render(ctx context.Context, assets fs.FS, in Payload) ([]byte, error) {
	if assets == nil || !json.Valid(in.Document) || len(in.Document) > 1<<20 {
		return nil, errors.New("quote PDF assets or document unavailable")
	}
	if _, err := fs.Stat(assets, "quote-print.html"); err != nil {
		return nil, errors.New("quote PDF print entry unavailable")
	}
	if !Available() {
		return nil, errors.New("Chromium unavailable")
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	host := listener.Addr().String()
	fileServer := http.FileServer(http.FS(assets))
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'")
		if r.Host != host || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		switch {
		case r.URL.Path == "/payload":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(payload)
		case r.URL.Path == "/quote-print.html" || strings.HasPrefix(r.URL.Path, "/assets/"):
			fileServer.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/quote-profiles/assets/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/quote-profiles/assets/")
			asset, ok := in.ProfileAssets[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", asset.ContentType)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(asset.Bytes)
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	home, err := os.MkdirTemp("", "aeon-quote-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.DisableGPU, chromedp.NoSandbox, chromedp.UserDataDir(filepath.Join(home, "profile")))
	// Chromium must not inherit database, OIDC, or future SMTP credentials.
	opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		original := append([]string{}, cmd.Args...)
		cmd.Path = "/usr/bin/env"
		cmd.Args = append([]string{"env", "-i", "PATH=" + os.Getenv("PATH"), "HOME=" + home,
			"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache")}, original...)
	}))
	allocator, closeAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAllocator()
	browser, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	var result []byte
	var state struct {
		Ready    bool   `json:"ready"`
		Overflow string `json:"overflow"`
	}
	err = chromedp.Run(browser,
		chromedp.Navigate("http://"+host+"/quote-print.html"),
		chromedp.Evaluate(`new Promise(resolve => {
		  const root = document.documentElement;
		  const read = () => ({ready: root.dataset.pdfReady === 'true', overflow: root.dataset.pdfOverflow || ''});
		  const observer = new MutationObserver(() => { const s = read(); if (s.ready || s.overflow) { observer.disconnect(); resolve(s); } });
		  observer.observe(root, {attributes: true, attributeFilter: ['data-pdf-ready','data-pdf-overflow']});
		  const s = read(); if (s.ready || s.overflow) { observer.disconnect(); resolve(s); }
		})`, &state, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) }),
	)
	if err != nil {
		return nil, fmt.Errorf("quote PDF render failed: %w", err)
	}
	if !state.Ready || state.Overflow != "" {
		return nil, fmt.Errorf("quote PDF overflow: %s", state.Overflow)
	}
	err = chromedp.Run(browser, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		result, _, err = page.PrintToPDF().WithPrintBackground(true).WithPreferCSSPageSize(true).WithDisplayHeaderFooter(false).Do(ctx)
		return err
	}))
	if err != nil || len(result) < 100 || len(result) > 20<<20 || !strings.HasPrefix(string(result), "%PDF-") {
		return nil, errors.New("quote PDF output invalid")
	}
	return result, nil
}
