// SPDX-License-Identifier: AGPL-3.0-only

// Package version exposes the build's calendar version (inspr-calendar-v2).
// The release pipeline injects Version via -ldflags; development builds report "dev".
package version

// Version is the canonical calendar version (YYMMDDhhmmss.0.0) or "dev".
var Version = "dev"

// Scheme is the machine version scheme identifier.
const Scheme = "inspr-calendar-v2"
