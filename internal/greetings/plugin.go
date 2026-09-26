// SPDX-License-Identifier: AGPL-3.0-only

package greetings

import (
	"context"

	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
)

// ManifestPlugin declares the optional welcome panel to the compiled plugin
// registry. The coordinator registers this constructor in plugins.Builtin;
// GET /api/me/greeting is mounted separately through New and remains available
// to an authenticated principal without a tenant plugin installation.
func ManifestPlugin() (plugins.Plugin, error) {
	p := plugins.Plugin{
		Manifest: plugins.Manifest{
			ID: "greetings", Version: "1", Owner: "aeon",
			Permissions: []string{fence.PermViewsProvide},
			Views:       []plugins.View{{ID: "greeting", Panels: []string{"welcome"}}},
		},
		Views: welcomeView{},
	}
	sum, err := plugins.Digest(p)
	if err != nil {
		return plugins.Plugin{}, err
	}
	p.Manifest.DigestSHA256 = sum
	return p, nil
}

type welcomeView struct{}

func (welcomeView) Views(_ context.Context, _ plugins.Call) ([]plugins.View, error) {
	return []plugins.View{{ID: "greeting", Panels: []string{"welcome"}}}, nil
}
