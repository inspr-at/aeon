// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/inspr-at/paimos/internal/brand"
	"github.com/inspr-at/paimos/internal/version"
)

type healthBody struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

type versionBody struct {
	Version string      `json:"version"`
	Scheme  string      `json:"scheme"`
	Brand   brand.Brand `json:"brand"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbState := "down"
	if s.Pool != nil {
		if err := s.Pool.Ping(r.Context()); err != nil {
			slog.Error("database ping failed", "err", err)
		} else {
			dbState = "ok"
		}
	}
	WriteJSON(w, http.StatusOK, healthBody{Status: "ok", DB: dbState})
}

// handleVersion answers the build's calendar version and the product's names
// (brand.json, or the deployment's AEON_BRAND_FILE when the server was given one).
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, versionBody{Version: version.Version, Scheme: version.Scheme, Brand: s.brand()})
}

// brand is the deployment's brand, else the embedded brand.json.
func (s *Server) brand() brand.Brand {
	if s.Brand != nil {
		return *s.Brand
	}
	return brand.Default()
}
