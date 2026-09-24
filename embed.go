// SPDX-License-Identifier: AGPL-3.0-only

// Package aeon holds files at the repository root that the binary embeds.
package aeon

import _ "embed"

// BrandJSON is brand.json (schema inspr.brand.v1): the product's names.
//
//go:embed brand.json
var BrandJSON []byte
