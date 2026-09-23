// SPDX-License-Identifier: AGPL-3.0-only

//go:build webembed

package web

import "io/fs"

// Static returns the embedded dist tree. The embed root contains dist/.
func Static() (fs.FS, bool) {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}
