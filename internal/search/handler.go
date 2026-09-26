// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/inspr-at/paimos/internal/httpapi"
	"github.com/inspr-at/paimos/internal/tenant"
)

type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

func errBad(msg string) error { return badRequest{msg: msg} }

func (m *Module) handleSearch(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok || p.ID == "" || p.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	in, err := parseQuery(r.URL.Query())
	if err != nil {
		var br badRequest
		if errors.As(err, &br) {
			writeError(w, http.StatusBadRequest, "invalid_request", br.msg)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid search")
		return
	}
	page, err := m.search(r.Context(), p, in)
	if err != nil {
		var br badRequest
		if errors.As(err, &br) {
			writeError(w, http.StatusBadRequest, "invalid_request", br.msg)
			return
		}
		slog.Error("search", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, http.StatusOK, page)
}

func parseQuery(vals url.Values) (queryInput, error) {
	if !vals.Has("q") {
		return queryInput{}, errBad("q is required")
	}
	q := strings.TrimSpace(vals.Get("q"))
	if q == "" {
		return queryInput{}, errBad("q is required")
	}
	if len(q) > 1000 {
		return queryInput{}, errBad("q is too long")
	}
	kind, err := optionalUUID(vals, "kind_id")
	if err != nil {
		return queryInput{}, err
	}
	state, err := optionalState(vals)
	if err != nil {
		return queryInput{}, err
	}
	limit, err := parseLimit(vals.Get("limit"))
	if err != nil {
		return queryInput{}, err
	}
	cursor := vals.Get("cursor")
	if len(cursor) > 2048 {
		return queryInput{}, errBad("cursor does not match this search")
	}
	return queryInput{Q: q, Kind: kind, State: state, Limit: limit, Cursor: cursor}, nil
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 200 {
		return 0, errBad("limit must be from 1 to 200")
	}
	return n, nil
}

func optionalUUID(vals url.Values, name string) (string, error) {
	if !vals.Has(name) {
		return "", nil
	}
	raw := strings.TrimSpace(vals.Get(name))
	id, ok := parseUUID(raw)
	if !ok {
		return "", errBad(name + " is invalid")
	}
	return id, nil
}

func optionalState(vals url.Values) (string, error) {
	if !vals.Has("state") {
		return "", nil
	}
	state := strings.TrimSpace(vals.Get("state"))
	if state == "" || len(state) > 200 {
		return "", errBad("state is invalid")
	}
	return state, nil
}

func parseUUID(s string) (string, bool) {
	var u pgtype.UUID
	if len(s) != 36 || u.Scan(s) != nil || !u.Valid {
		return "", false
	}
	return strings.ToLower(s), true
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	httpapi.WriteJSON(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
}
