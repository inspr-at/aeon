// SPDX-License-Identifier: AGPL-3.0-only
//go:build !darwin

package agentd

import (
	"context"
	"errors"
)

func (*GrokAdapter) startNative(context.Context, StartRequest, GrokBinding, func(AdapterEvent)) (Process, error) {
	return nil, errors.New("native Grok isolation requires macOS")
}
func (*GrokAdapter) probeNative(context.Context, GrokBinding) bool { return false }
