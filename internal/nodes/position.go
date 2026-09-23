// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"math/big"
	"strconv"
	"strings"
)

// sibling is one live child ordered by (position, id).
type sibling struct {
	ID       string
	Position string
}

// Place inserts self among siblings. beforeID nil appends. The returned
// updates renumber other siblings only when the decimal rank has no room
// left in numeric(30,15).
func Place(siblings []sibling, selfID string, beforeID *string) (string, []sibling, error) {
	filtered := make([]sibling, 0, len(siblings))
	for _, s := range siblings {
		if s.ID == selfID {
			continue
		}
		filtered = append(filtered, s)
	}
	idx := len(filtered)
	if beforeID != nil {
		found := false
		for i, s := range filtered {
			if s.ID == *beforeID {
				idx = i
				found = true
				break
			}
		}
		if !found {
			return "", nil, badRequest("before_id is not a sibling")
		}
	}
	if pos, ok := gapPosition(filtered, idx); ok {
		return pos, nil, nil
	}
	return renumber(filtered, selfID, idx)
}

func gapPosition(siblings []sibling, idx int) (string, bool) {
	switch {
	case len(siblings) == 0:
		return "0", true
	case idx == 0:
		return shift(siblings[0].Position, -1)
	case idx == len(siblings):
		return shift(siblings[len(siblings)-1].Position, 1)
	default:
		return midpoint(siblings[idx-1].Position, siblings[idx].Position)
	}
}

func renumber(siblings []sibling, selfID string, idx int) (string, []sibling, error) {
	seq := make([]sibling, 0, len(siblings)+1)
	seq = append(seq, siblings[:idx]...)
	seq = append(seq, sibling{ID: selfID})
	seq = append(seq, siblings[idx:]...)
	var selfPos string
	var updates []sibling
	for i, s := range seq {
		pos := strconv.Itoa(i)
		if s.ID == selfID {
			selfPos = pos
			continue
		}
		if !decimalEqual(s.Position, pos) {
			updates = append(updates, sibling{ID: s.ID, Position: pos})
		}
	}
	return selfPos, updates, nil
}

func shift(s string, delta int64) (string, bool) {
	r, ok := parseDecimal(s)
	if !ok {
		return "", false
	}
	r.Add(r, big.NewRat(delta, 1))
	return formatRat(r)
}

func midpoint(a, b string) (string, bool) {
	ar, aok := parseDecimal(a)
	br, bok := parseDecimal(b)
	if !aok || !bok {
		return "", false
	}
	sum := new(big.Rat).Add(ar, br)
	mid := sum.Quo(sum, big.NewRat(2, 1))
	return formatRat(mid)
}

func decimalEqual(a, b string) bool {
	ar, aok := parseDecimal(a)
	br, bok := parseDecimal(b)
	return aok && bok && ar.Cmp(br) == 0
}

func parseDecimal(s string) (*big.Rat, bool) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	return r, ok
}

// numeric(30,15): 15 digits before the decimal point and 15 after.
func formatRat(r *big.Rat) (string, bool) {
	if r == nil || !fitsNumeric(r) {
		return "", false
	}
	return trimZeros(r.FloatString(15)), true
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func fitsNumeric(r *big.Rat) bool {
	abs := new(big.Rat).Abs(r)
	limit := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(15), nil))
	if abs.Cmp(limit) >= 0 {
		return false
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(15), nil)
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(scale))
	return scaled.IsInt()
}

func trimDecimal(s string) string {
	r, ok := parseDecimal(s)
	if !ok {
		return s
	}
	if formatted, ok := formatRat(r); ok {
		return formatted
	}
	return s
}
