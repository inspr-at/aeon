// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"encoding/json"
	"math/big"
)

// usdMicros converts a vendor JSON decimal to integer microdollars. The
// decimal is never passed through a binary float in AEON's accounting path.
func usdMicros(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || len(raw) > 64 || raw[0] == '"' || string(raw) == "null" {
		return 0, false
	}
	value, ok := new(big.Rat).SetString(string(raw))
	if !ok || value.Sign() < 0 {
		return 0, false
	}
	value.Mul(value, big.NewRat(1_000_000, 1))
	// Vendor amounts can have sub-micro precision. Round half up once on
	// each cumulative snapshot, then subtract snapshots in integer units.
	n := new(big.Int).Quo(value.Num(), value.Denom())
	remainder := new(big.Int).Rem(value.Num(), value.Denom())
	if remainder.Mul(remainder, big.NewInt(2)).Cmp(value.Denom()) >= 0 {
		n.Add(n, big.NewInt(1))
	}
	if !n.IsInt64() {
		return 0, false
	}
	return n.Int64(), true
}

func cumulativeDelta(current int64, previous *int64) int64 {
	if current < 0 || current < *previous {
		return 0
	}
	delta := current - *previous
	*previous = current
	return delta
}
