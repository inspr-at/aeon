// SPDX-License-Identifier: AGPL-3.0-only

package views

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/tenant"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type viewSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type savedView struct {
	ID             string          `json:"id"`
	OwnerPrincipal string          `json:"owner_principal_id"`
	Name           string          `json:"name"`
	Filters        json.RawMessage `json:"filters"`
	Sort           viewSort        `json:"sort"`
	Columns        []string        `json:"columns"`
	Shared         bool            `json:"shared"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type viewWrite struct {
	Name    string          `json:"name"`
	Filters json.RawMessage `json:"filters"`
	Sort    viewSort        `json:"sort"`
	Columns []string        `json:"columns"`
	Shared  bool            `json:"shared"`
}

type viewPatch struct {
	Name    *string          `json:"name"`
	Filters *json.RawMessage `json:"filters"`
	Sort    *viewSort        `json:"sort"`
	Columns *[]string        `json:"columns"`
	Shared  *bool            `json:"shared"`
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var items []savedView
	err := m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT id::text, owner_principal_id::text, name, filters, sort_field, sort_direction,
			       columns, shared, created_at, updated_at
			FROM saved_views
			WHERE owner_principal_id = $1::uuid OR shared
			ORDER BY updated_at DESC, id`, p.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		items = make([]savedView, 0)
		for rows.Next() {
			v, err := scanView(rows)
			if err != nil {
				return err
			}
			items = append(items, v)
		}
		return rows.Err()
	})
	if err != nil {
		writeDBError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, struct {
		Items []savedView `json:"items"`
	}{items})
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var in viewWrite
	if err := decodeJSON(w, r, &in); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateWrite(in); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var result savedView
	err := m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		result, err = insertView(r, tx, p.TenantID, p.ID, in)
		if err != nil {
			return err
		}
		return m.eventSink.Append(r.Context(), tx, p.ID, "view.created", nil, result)
	})
	if err != nil {
		writeDBError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, result)
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.PathValue("viewId")
	if !uuidPattern.MatchString(id) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	var result savedView
	err := m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		result, err = selectView(r, tx, `id = $1::uuid AND (owner_principal_id = $2::uuid OR shared)`, id, p.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	if err != nil {
		writeDBError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (m *Module) patch(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.PathValue("viewId")
	if !uuidPattern.MatchString(id) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	var in viewPatch
	if err := decodeJSON(w, r, &in); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePatch(in); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var result savedView
	var forbidden bool
	err := m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		before, err := selectView(r, tx, `id = $1::uuid`, id)
		if err != nil {
			return err
		}
		if before.OwnerPrincipal != p.ID {
			forbidden = true
			return nil
		}
		result, err = updateView(r, tx, id, before, in)
		if err != nil {
			return err
		}
		return m.eventSink.Append(r.Context(), tx, p.ID, "view.updated", before, result)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	if err != nil {
		writeDBError(w, err)
		return
	}
	if forbidden {
		httpapi.WriteError(w, http.StatusForbidden, "only the owner can update this view")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.PathValue("viewId")
	if !uuidPattern.MatchString(id) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	var forbidden bool
	err := m.inTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		before, err := selectView(r, tx, `id = $1::uuid`, id)
		if err != nil {
			return err
		}
		if before.OwnerPrincipal != p.ID {
			forbidden = true
			return nil
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM saved_views WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return m.eventSink.Append(r.Context(), tx, p.ID, "view.deleted", before, nil)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	if err != nil {
		writeDBError(w, err)
		return
	}
	if forbidden {
		httpapi.WriteError(w, http.StatusForbidden, "only the owner can delete this view")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type rowScanner interface {
	Scan(...any) error
}

func scanView(row rowScanner) (savedView, error) {
	var v savedView
	var filters []byte
	err := row.Scan(&v.ID, &v.OwnerPrincipal, &v.Name, &filters, &v.Sort.Field,
		&v.Sort.Direction, &v.Columns, &v.Shared, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return savedView{}, err
	}
	v.Filters = json.RawMessage(filters)
	return v, nil
}

const viewColumns = `id::text, owner_principal_id::text, name, filters, sort_field, sort_direction,
                     columns, shared, created_at, updated_at`

func selectView(r *http.Request, tx pgx.Tx, where string, args ...any) (savedView, error) {
	return scanView(tx.QueryRow(r.Context(), `SELECT `+viewColumns+` FROM saved_views WHERE `+where, args...))
}

func insertView(r *http.Request, tx pgx.Tx, tenantID, ownerID string, in viewWrite) (savedView, error) {
	return scanView(tx.QueryRow(r.Context(), `
		INSERT INTO saved_views (tenant_id, owner_principal_id, name, filters, sort_field, sort_direction, columns, shared)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, $5, $6, $7, $8)
		RETURNING `+viewColumns,
		tenantID, ownerID, strings.TrimSpace(in.Name), []byte(in.Filters), in.Sort.Field, in.Sort.Direction, in.Columns, in.Shared))
}

func updateView(r *http.Request, tx pgx.Tx, id string, before savedView, in viewPatch) (savedView, error) {
	after := before
	if in.Name != nil {
		after.Name = strings.TrimSpace(*in.Name)
	}
	if in.Filters != nil {
		after.Filters = *in.Filters
	}
	if in.Sort != nil {
		after.Sort = *in.Sort
	}
	if in.Columns != nil {
		after.Columns = *in.Columns
	}
	if in.Shared != nil {
		after.Shared = *in.Shared
	}
	return scanView(tx.QueryRow(r.Context(), `
		UPDATE saved_views SET name = $2, filters = $3::jsonb, sort_field = $4, sort_direction = $5,
		       columns = $6, shared = $7, updated_at = now()
		WHERE id = $1::uuid
		RETURNING `+viewColumns,
		id, after.Name, []byte(after.Filters), after.Sort.Field, after.Sort.Direction, after.Columns, after.Shared))
}

func validateWrite(in viewWrite) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateFilters(in.Filters); err != nil {
		return err
	}
	if err := validateSort(in.Sort); err != nil {
		return err
	}
	if in.Columns == nil {
		return errors.New("columns is required")
	}
	return nil
}

func validatePatch(in viewPatch) error {
	if in.Name == nil && in.Filters == nil && in.Sort == nil && in.Columns == nil && in.Shared == nil {
		return errors.New("at least one property is required")
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return errors.New("name must not be empty")
	}
	if in.Filters != nil {
		if err := validateFilters(*in.Filters); err != nil {
			return err
		}
	}
	if in.Sort != nil {
		if err := validateSort(*in.Sort); err != nil {
			return err
		}
	}
	return nil
}

func validateFilters(filters json.RawMessage) error {
	var value map[string]json.RawMessage
	if len(filters) == 0 || json.Unmarshal(filters, &value) != nil || value == nil {
		return errors.New("filters must be an object")
	}
	return nil
}

func validateSort(s viewSort) error {
	switch s.Field {
	case "position", "updated_at", "created_at", "key", "title":
	default:
		return errors.New("sort.field is invalid")
	}
	if s.Direction != "asc" && s.Direction != "desc" {
		return errors.New("sort.direction is invalid")
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON request body")
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, http.StatusNotFound, "view not found")
		return
	}
	httpapi.WriteError(w, http.StatusInternalServerError, "database operation failed")
}
