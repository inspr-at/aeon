// SPDX-License-Identifier: AGPL-3.0-only

// Package demo seeds a tenant with fictional screenshot data.
//
// Run is the CLI constructor. The coordinator wires it. This package does
// not edit cmd/aeon, web/src/router.ts, or internal/plugins/builtin.go, and
// it does not export an httpapi.Module or a plugin manifest.
//
//	if len(os.Args) > 1 && os.Args[1] == "demo" {
//	    err := withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
//	        return demo.Run(ctx, pool, os.Args[2:], os.Stdout)
//	    })
//	}
//
// Seed is the same work for tests. Both refuse unless AEON_ENV is dev.
// Writes go through the existing HTTP modules, inside db.InTenant, and each
// module appends the usual tenant event.
package demo
