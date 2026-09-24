// SPDX-License-Identifier: AGPL-3.0-only
package attachments

import "github.com/inspr-at/aeon/internal/plugins"

// Plugin declares attachments to the compiled plugin registry. The coordinator
// passes this constructor to plugins.Builtin; the HTTP module mounts separately.
func Plugin() (plugins.Plugin, error) {
	p := plugins.Plugin{Manifest: plugins.Manifest{ID: "attachments", Version: "1.0.0", Owner: "aeon"}}
	digest, err := plugins.Digest(p)
	if err != nil {
		return plugins.Plugin{}, err
	}
	p.Manifest.DigestSHA256 = digest
	return p, nil
}
