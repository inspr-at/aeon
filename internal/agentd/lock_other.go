// SPDX-License-Identifier: AGPL-3.0-only
//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package agentd

import (
	"errors"
	"os"
)

func acquireInstanceLock(string, string) (*os.File, error) {
	return nil, errors.New("agentd process ownership unsupported on this platform")
}
