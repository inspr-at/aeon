// SPDX-License-Identifier: AGPL-3.0-only

//go:build !webembed

package web

import "io/fs"

// Static reports that this build has no embedded web assets.
// Build with -tags webembed to embed web/dist via embed.go.
func Static() (fs.FS, bool) {
	return nil, false
}
