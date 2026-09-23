// SPDX-License-Identifier: AGPL-3.0-only

package cli

import "fmt"

// exitError carries a process status. 2 is usage, 3 is not available yet.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &exitError{code: 2, msg: fmt.Sprintf(format, args...)}
}

func notYet(msg string) error {
	return &exitError{code: 3, msg: msg}
}
