// SPDX-License-Identifier: AGPL-3.0-only
package profile

import "github.com/inspr-at/aeon/internal/plugins"

// Plugin declares the compiled personal-profile module. The coordinator passes
// this constructor to plugins.Builtin; New mounts its HTTP routes separately.
func Plugin() (plugins.Plugin, error) {
	p := plugins.Plugin{Manifest: plugins.Manifest{ID: "personal_profile", Version: "1.0.0", Owner: "aeon"}}
	digest, err := plugins.Digest(p)
	if err != nil {
		return plugins.Plugin{}, err
	}
	p.Manifest.DigestSHA256 = digest
	return p, nil
}
