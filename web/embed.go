// SPDX-License-Identifier: AGPL-3.0-only

//go:build webembed

package web

import "embed"

// Dist is the built Vue app (web/dist), included only with -tags webembed.
//
//go:embed all:dist
var Dist embed.FS
