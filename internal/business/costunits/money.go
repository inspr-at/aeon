// SPDX-License-Identifier: AGPL-3.0-only

package costunits

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

const (
	amountScale  = 4
	amountDigits = 18
)

// Decimal is an exact non-negative money amount with at most four fractional
// digits. JSON encoding writes a number, not a string and not a float.
type Decimal struct {
	text string
}

func (d Decimal) String() string { return d.text }

func (d Decimal) MarshalJSON() ([]byte, error) {
	if !canonicalAmount(d.text) {
		return nil, errors.New("invalid amount")
	}
	return []byte(d.text), nil
}

func (d *Decimal) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || raw[0] == '"' {
		return errAmount
	}
	parsed, err := parseAmount(json.Number(raw))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d Decimal) equal(other Decimal) bool { return d.text == other.text }

var (
	errAmount = errors.New("amounts must be non-negative decimals with at most 4 fractional digits")
	errDate   = errors.New("date must be YYYY-MM-DD")
)

func parseAmount(n json.Number) (Decimal, error) {
	raw := n.String()
	if raw == "" || strings.ContainsAny(raw, " \t/") {
		return Decimal{}, errAmount
	}
	mantissa, exp, err := splitExponent(raw)
	if err != nil {
		return Decimal{}, err
	}
	if strings.Contains(mantissa, "+") {
		return Decimal{}, errAmount
	}
	rat, ok := new(big.Rat).SetString(mantissa)
	if !ok {
		return Decimal{}, errAmount
	}
	if exp != 0 {
		pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(exp))), nil)
		factor := new(big.Rat).SetInt(pow)
		if exp > 0 {
			rat.Mul(rat, factor)
		} else {
			rat.Quo(rat, factor)
		}
	}
	if rat.Sign() < 0 {
		return Decimal{}, errAmount
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(amountScale), nil)
	scaled := new(big.Rat).Mul(rat, new(big.Rat).SetInt(scale))
	if !scaled.IsInt() {
		return Decimal{}, errAmount
	}
	minor := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(amountDigits), nil)
	if minor.Cmp(limit) >= 0 {
		return Decimal{}, errAmount
	}
	text := formatMinor(minor)
	if !canonicalAmount(text) {
		return Decimal{}, errAmount
	}
	return Decimal{text: text}, nil
}

func splitExponent(s string) (string, int, error) {
	idx := strings.IndexAny(s, "eE")
	if idx < 0 {
		return s, 0, nil
	}
	mantissa := s[:idx]
	rest := s[idx+1:]
	if mantissa == "" || mantissa == "-" || mantissa == "." || rest == "" {
		return "", 0, errAmount
	}
	sign := 1
	if rest[0] == '+' || rest[0] == '-' {
		if rest[0] == '-' {
			sign = -1
		}
		rest = rest[1:]
	}
	if rest == "" || len(rest) > 2 {
		return "", 0, errAmount
	}
	n := 0
	for _, c := range rest {
		if c < '0' || c > '9' {
			return "", 0, errAmount
		}
		n = n*10 + int(c-'0')
	}
	exp := sign * n
	if exp > 18 || exp < -18 {
		return "", 0, errAmount
	}
	return mantissa, exp, nil
}

func formatMinor(minor *big.Int) string {
	digits := minor.Text(10)
	if len(digits) <= amountScale {
		digits = strings.Repeat("0", amountScale+1-len(digits)) + digits
	}
	whole := digits[:len(digits)-amountScale]
	frac := strings.TrimRight(digits[len(digits)-amountScale:], "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

func canonicalAmount(s string) bool {
	if s == "0" {
		return true
	}
	whole, frac, found := strings.Cut(s, ".")
	if whole == "" || (len(whole) > 1 && whole[0] == '0') {
		return false
	}
	for _, c := range whole {
		if c < '0' || c > '9' {
			return false
		}
	}
	if !found {
		return true
	}
	if frac == "" || len(frac) > amountScale || frac[len(frac)-1] == '0' {
		return false
	}
	for _, c := range frac {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func parseDate(s string) (string, error) {
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil || parsed.Format("2006-01-02") != s {
		return "", errDate
	}
	return s, nil
}

func canonicalJSON(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}
