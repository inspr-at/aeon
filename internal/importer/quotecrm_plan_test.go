// SPDX-License-Identifier: AGPL-3.0-only

package importer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSyntheticQuoteCRMMapping(t *testing.T) {
	raw, err := os.ReadFile("../../web/tests/quotes/fixtures/import-synthetic.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture QuoteCRMSnapshot
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	got, err := PlanQuoteCRMMapping(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].SourceKey != "pma:synthetic-one:customer:17" || got[3].SourceKey != "pma:synthetic-one:quote:9" || got[3].Review != "original accepted PDF unavailable" {
		t.Fatalf("unexpected mapping: %+v", got)
	}
	other := fixture
	other.SourceInstance = "synthetic-two"
	second, err := PlanQuoteCRMMapping(other)
	if err != nil || second[0].SourceKey == got[0].SourceKey {
		t.Fatal("source namespace collision", err)
	}
	bad := fixture
	bad.Contacts = append([]QuoteCRMContact(nil), fixture.Contacts...)
	bad.Contacts[0].CustomerID = "unknown"
	if _, err := PlanQuoteCRMMapping(bad); err == nil {
		t.Fatal("orphan contact accepted")
	}
	bad = fixture
	bad.Quotes = append([]QuoteCRMQuote(nil), fixture.Quotes...)
	bad.Quotes[0].NetTotalMinor = -1
	if _, err := PlanQuoteCRMMapping(bad); err == nil {
		t.Fatal("negative minor units accepted")
	}
	bad = fixture
	bad.Quotes = append([]QuoteCRMQuote(nil), fixture.Quotes...)
	bad.Quotes[0].OriginalSHA256 = ""
	planned, err := PlanQuoteCRMMapping(bad)
	if err != nil || !strings.Contains(planned[3].Review, "digest unavailable") {
		t.Fatal("missing original evidence not flagged", err)
	}
	bad.Quotes[0].OriginalSHA256 = strings.Repeat("x", 64)
	if _, err := PlanQuoteCRMMapping(bad); err == nil {
		t.Fatal("malformed historical digest accepted")
	}
	bad.Quotes[0].OriginalSHA256 = strings.Repeat("a", 64)
	bad.Quotes[0].State = "declined"
	planned, err = PlanQuoteCRMMapping(bad)
	if err != nil || !strings.Contains(planned[3].Review, "no native decision is synthesized") {
		t.Fatal("historical declined state lost", err)
	}
}
