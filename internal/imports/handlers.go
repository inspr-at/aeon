// SPDX-License-Identifier: AGPL-3.0-only

package imports

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/authz"
	"github.com/inspr-at/paimos/internal/httpapi"
	"github.com/inspr-at/paimos/internal/tenant"
)

type importJob struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	ProcessedCount int64      `json:"processed_count"`
	CreatedCount   int64      `json:"created_count"`
	ErrorCount     int64      `json:"error_count"`
	FailureSummary *string    `json:"failure_summary"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

type pageCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	p, ok := m.importAdmin(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	items := make([]importJob, 0, limit)
	var hasMore bool
	err = m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		query := `SELECT id::text, status, source, processed_count, created_count, error_count,
		                 failure_summary, created_at, started_at, finished_at
		          FROM import_jobs`
		args := []any{}
		if cursor != nil {
			query += ` WHERE (created_at, id) < ($1, $2::uuid)`
			args = append(args, cursor.CreatedAt, cursor.ID)
		}
		query += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(args)+1)
		args = append(args, limit+1)
		rows, err := tx.Query(r.Context(), query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var job importJob
			if err := rows.Scan(&job.ID, &job.Status, &job.Source, &job.ProcessedCount,
				&job.CreatedCount, &job.ErrorCount, &job.FailureSummary,
				&job.CreatedAt, &job.StartedAt, &job.FinishedAt); err != nil {
				return err
			}
			items = append(items, job)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) > limit {
			hasMore = true
			items = items[:limit]
		}
		return nil
	})
	if err != nil {
		writeDBError(w)
		return
	}
	var next any
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = encodeCursor(pageCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	httpapi.WriteJSON(w, http.StatusOK, struct {
		Items      []importJob `json:"items"`
		NextCursor any         `json:"next_cursor"`
	}{items, next})
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	p, ok := m.importAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("importId")
	if !isUUID(id) {
		httpapi.WriteError(w, http.StatusNotFound, "import not found")
		return
	}
	var result importJob
	err := m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(), `
			SELECT id::text, status, source, processed_count, created_count, error_count,
			       failure_summary, created_at, started_at, finished_at
			FROM import_jobs WHERE id = $1::uuid`, id).Scan(
			&result.ID, &result.Status, &result.Source, &result.ProcessedCount,
			&result.CreatedCount, &result.ErrorCount, &result.FailureSummary,
			&result.CreatedAt, &result.StartedAt, &result.FinishedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, http.StatusNotFound, "import not found")
		return
	}
	if err != nil {
		writeDBError(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (m *Module) importAdmin(w http.ResponseWriter, r *http.Request) (tenant.Principal, bool) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return p, false
	}
	if p.Kind != tenant.Person || authz.Require(authz.BindPool(r.Context(), m.pool), "imports.read", authz.Scope{}) != nil {
		httpapi.WriteError(w, http.StatusForbidden, "administrator required")
		return p, false
	}
	return p, true
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 200 {
		return 0, errors.New("limit must be between 1 and 200")
	}
	return n, nil
}

func decodeCursor(raw string) (*pageCursor, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 1024 {
		return nil, errors.New("cursor too long")
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var cursor pageCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.CreatedAt.IsZero() || !isUUID(cursor.ID) {
		return nil, errors.New("invalid cursor payload")
	}
	return &cursor, nil
}

func encodeCursor(cursor pageCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
		} else if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func writeDBError(w http.ResponseWriter) {
	httpapi.WriteError(w, http.StatusInternalServerError, "database operation failed")
}
