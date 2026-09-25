// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type requestIDKey struct{}

const requestIDHeader = "X-Request-ID"

// RequestID returns the id set by the request-id middleware, if any.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func commonMiddleware(next http.Handler) http.Handler {
	return requestIDMiddleware(recoverMiddleware(securityMiddleware(next)))
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// blob: and data: images let the avatar crop dialog preview a local file
		// before upload; everything else stays same-origin.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Caller-supplied IDs can contain tokens. Generate our own trace value
		// before it enters response headers, context or structured logs.
		id := newRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		r = r.WithContext(ctx)
		w.Header().Set(requestIDHeader, id)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			slog.Info("request",
				"method", r.Method,
				"path", requestLogRoute(r),
				"status", sw.status,
				"request_id", id,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()
		next.ServeHTTP(sw, r)
	})
}

// routePatternMiddleware resolves the inner API mux before other middleware
// can clone the request or reject it. ServeMux normally sets Pattern while
// serving, but a later WithContext would hide that write from the logger.
func routePatternMiddleware(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, r.Pattern = mux.Handler(r)
		next.ServeHTTP(w, r)
	})
}

func requestLogRoute(r *http.Request) string {
	// Root and API catchalls identify no specific route. Use a sanitized path
	// for those, including unmatched public URLs and SPA capability links.
	if r.Pattern != "" && r.Pattern != "/" && r.Pattern != "/api" && r.Pattern != "/api/" {
		return r.Pattern
	}
	return redactedLogPath(r.URL.Path)
}

func redactedLogPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return "/"
	}
	// On an unmatched route there is no schema to distinguish an ID from a
	// capability. Keep only known static prefixes and redact every later
	// segment; this also covers future invite, link, confirmation and download
	// token shapes without having to recognize their exact spelling.
	if len(parts) >= 2 && strings.EqualFold(parts[0], "api") {
		switch strings.ToLower(parts[1]) {
		case "auth", "nodes", "quotes", "attachments", "public", "invites", "invite", "links", "link", "confirmation", "confirm", "downloads", "download":
			return "/api/" + strings.ToLower(parts[1]) + "/{redacted}"
		default:
			return "/api/{redacted}"
		}
	}
	if len(parts) == 1 && (parts[0] == "favicon.ico" || parts[0] == "robots.txt") {
		return path
	}
	switch strings.ToLower(parts[0]) {
	case "q", "offers", "invite", "invites", "link", "links", "confirm", "confirmation", "download", "downloads":
		return "/" + strings.ToLower(parts[0]) + "/{redacted}"
	default:
		return "/{redacted}"
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// Both the URL path and the panic value can contain a public
				// capability or an upstream credential. The request ID is enough
				// to correlate this failure without disclosing either value.
				slog.Error("panic", "request_id", RequestID(r.Context()))
				WriteError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
