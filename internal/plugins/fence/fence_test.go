// SPDX-License-Identifier: AGPL-3.0-only

package fence

import "testing"

func TestSealRejectsEmptyAndOptionalSets(t *testing.T) {
	if _, err := Seal(nil); err == nil {
		t.Fatal("empty set proved a prerequisite")
	}
	_, err := Seal([]Check{{Kind: KindAuthorization, Required: false, Succeeded: true}})
	if err == nil {
		t.Fatal("optional-only set proved a prerequisite")
	}
}

func TestSealIsOrderIndependent(t *testing.T) {
	left := []Check{
		{Kind: KindCredentialHandoff, Required: true, Succeeded: true},
		{Kind: KindAuthorization, Required: true, Succeeded: false},
	}
	right := []Check{left[1], left[0]}
	a, err := Seal(left)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Seal(right)
	if err != nil {
		t.Fatal(err)
	}
	if a != b || len(a) != 64 {
		t.Fatalf("seal = %s vs %s", a, b)
	}
}
