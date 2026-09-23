// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import "testing"

func TestPlaceRanks(t *testing.T) {
	pos, updates, err := Place(nil, "", nil)
	if err != nil || pos != "0" || len(updates) != 0 {
		t.Fatalf("empty append: pos=%s updates=%v err=%v", pos, updates, err)
	}

	sibs := []sibling{{ID: "a", Position: "0"}, {ID: "b", Position: "1"}}
	beforeB := "b"
	pos, updates, err = Place(sibs, "", &beforeB)
	if err != nil || pos != "0.5" || len(updates) != 0 {
		t.Fatalf("between: pos=%s updates=%v err=%v", pos, updates, err)
	}
	pos, updates, err = Place(sibs, "", nil)
	if err != nil || pos != "2" || len(updates) != 0 {
		t.Fatalf("append: pos=%s updates=%v err=%v", pos, updates, err)
	}
	beforeA := "a"
	pos, updates, err = Place(sibs, "", &beforeA)
	if err != nil || pos != "-1" || len(updates) != 0 {
		t.Fatalf("before first: pos=%s updates=%v err=%v", pos, updates, err)
	}

	tiny := []sibling{{ID: "a", Position: "0"}, {ID: "b", Position: "0.000000000000001"}}
	pos, updates, err = Place(tiny, "self", &beforeB)
	if err != nil || pos != "1" || len(updates) != 1 || updates[0].ID != "b" || updates[0].Position != "2" {
		t.Fatalf("renumber: pos=%s updates=%v err=%v", pos, updates, err)
	}

	if _, _, err := Place(sibs, "a", &beforeA); err == nil {
		t.Fatal("before self should fail")
	}
}

func TestTrimDecimal(t *testing.T) {
	if got := trimDecimal("0.000000000000000"); got != "0" {
		t.Fatalf("trim zero: %s", got)
	}
	if got := trimDecimal("-1.500000000000000"); got != "-1.5" {
		t.Fatalf("trim negative: %s", got)
	}
}
