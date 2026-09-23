// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}

// WriteError writes a JSON error body: {"error": message}.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func handleNotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotFound, "not found")
}
