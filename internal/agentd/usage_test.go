// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"encoding/json"
	"testing"
)

func TestUSDMicrosExactDecimal(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
		ok   bool
	}{
		{`0.0000125`, 13, true},
		{`7.00000049`, 7_000_000, true},
		{`1e-6`, 1, true},
		{`-0.01`, 0, false},
		{`null`, 0, false},
		{`100000000000000`, 0, false},
	} {
		got, ok := usdMicros(json.RawMessage(tc.raw))
		if got != tc.want || ok != tc.ok {
			t.Errorf("usdMicros(%s) = %d, %t; want %d, %t", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}
