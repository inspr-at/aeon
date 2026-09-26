// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/journey"
)

// journeyCommand is operator-only. These mutations have no HTTP route.
func journeyCommand(args []string, stdout io.Writer) error {
	if os.Getenv("AEON_ENV") != "dev" {
		return errors.New("journey operator commands require AEON_ENV=dev")
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		return journey.RunOperator(ctx, pool, args, stdout)
	})
}
