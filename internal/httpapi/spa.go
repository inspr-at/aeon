// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/inspr-at/aeon/internal/brand"
)

// placeholderHTML is served when the binary has no web build; it carries the
// deployment's wordmark like every other surface.
func placeholderHTML(b brand.Brand) []byte {
	name := html.EscapeString(b.Wordmark)
	return []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + name + `</title>
</head>
<body>
<p>` + name + `</p>
</body>
</html>
`)
}

func spaHandler(fsys fs.FS, b brand.Brand) http.Handler {
	if fsys == nil {
		page := placeholderHTML(b)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowRead(w, r) {
				return
			}
			writeHTML(w, r, page)
		})
	}
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowRead(w, r) {
			return
		}
		if name, ok := staticName(r.URL.Path); ok {
			if f, err := fsys.Open(name); err == nil {
				st, statErr := f.Stat()
				f.Close()
				if statErr == nil && !st.IsDir() {
					files.ServeHTTP(w, r)
					return
				}
			}
		}
		body, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writeHTML(w, r, body)
	})
}

func allowRead(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func staticName(urlPath string) (string, bool) {
	cleaned := path.Clean("/" + urlPath)
	if cleaned == "/" || cleaned == "." {
		return "", false
	}
	name := strings.TrimPrefix(cleaned, "/")
	if name == "" || name == ".." || strings.HasPrefix(name, "../") {
		return "", false
	}
	return name, true
}

func writeHTML(w http.ResponseWriter, r *http.Request, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
