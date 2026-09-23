// SPDX-License-Identifier: AGPL-3.0-only

package embedding

import (
	"errors"
	"math"
)

// Dimensions is the halfvec width fixed by the R1 schema.
const Dimensions = 1536

// Validate reports whether v is a finite query or document vector.
func Validate(v []float32) error {
	if len(v) != Dimensions {
		return errors.New("embedding must have 1536 dimensions")
	}
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return errors.New("embedding must be finite")
		}
	}
	return nil
}
