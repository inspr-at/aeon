// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// decimal stores exact ten-thousandths. JSON and SQL use decimal text, never
// binary floating point. Quote tax rates permit one further fractional digit.
type decimal int64

const maxAmount decimal = 999999999999999999 // numeric(18,4) scaled by 10,000

var decimalRe = regexp.MustCompile(`^([0-9]{1,14})(?:\.([0-9]{1,4}))?$`)
var taxRe = regexp.MustCompile(`^([0-9])(?:\.([0-9]{1,5}))?$`)

func parseDecimal(s string, rate bool) (decimal, error) {
	re := decimalRe
	places := 4
	if rate {
		re = taxRe
		places = 5
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid decimal")
	}
	whole := new(big.Int)
	whole.SetString(m[1], 10)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	whole.Mul(whole, scale)
	if m[2] != "" {
		frac := new(big.Int)
		frac.SetString(m[2]+strings.Repeat("0", places-len(m[2])), 10)
		whole.Add(whole, frac)
	}
	if !whole.IsInt64() || (!rate && whole.Int64() > int64(maxAmount)) {
		return 0, fmt.Errorf("decimal out of range")
	}
	if rate && whole.Int64() > 100000 {
		return 0, fmt.Errorf("tax rate out of range")
	}
	return decimal(whole.Int64()), nil
}
func (d decimal) String() string               { return fmt.Sprintf("%d.%04d", int64(d)/10000, int64(d)%10000) }
func (d decimal) MarshalJSON() ([]byte, error) { return []byte(d.String()), nil }
func (d *decimal) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || b[0] == '"' {
		return fmt.Errorf("decimal must be a number")
	}
	v, err := parseDecimal(string(b), false)
	*d = v
	return err
}
func multiply(a, b decimal, scale int64) (decimal, error) {
	n := new(big.Int).Mul(big.NewInt(int64(a)), big.NewInt(int64(b)))
	n.Add(n, big.NewInt(scale/2))
	n.Div(n, big.NewInt(scale))
	if !n.IsInt64() || n.Sign() < 0 || n.Int64() > int64(maxAmount) {
		return 0, fmt.Errorf("amount out of range")
	}
	return decimal(n.Int64()), nil
}
func add(a, b decimal) (decimal, error) {
	n := new(big.Int).Add(big.NewInt(int64(a)), big.NewInt(int64(b)))
	if !n.IsInt64() || n.Int64() > int64(maxAmount) {
		return 0, fmt.Errorf("amount out of range")
	}
	return decimal(n.Int64()), nil
}
func number(s string) json.Number { return json.Number(s) }
