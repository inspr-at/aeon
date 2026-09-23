// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"context"
	"os/signal"
	"syscall"
)

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
