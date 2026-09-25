// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"encoding/json"
	"fmt"
	"testing"
)

// U19 (AEON-110): a draft takes a profile's current revision from the title bar's
// picker, can go back to the standard document with an empty profile_id, and the
// choice is guarded like every other draft change.
func TestSelectProfileOnDraftAndReturnToStandard(t *testing.T) {
	f := newQuoteFixture(t, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "quotes-profile-select")
	settings := `{"expected_revision":0,"numbering_time_zone":"Europe/Vienna","default_currency":"EUR","sender":{"company":"Example Sender","street":"Example Street 1","postal_code":"0000","city":"Example City","country":"AT","email":"sender@example.test"},"defaults":{"intro":"","blocks":[],"accept_text":"","vat_note":""},"layout":{},"smtp_confirmation_enabled":false}`
	if status, body := f.call("admin", "PATCH", "/api/quotes/settings", settings); status != 200 {
		t.Fatalf("settings %d %v", status, body)
	}
	raw, _ := json.Marshal(profileWrite{Name: "Beispiel Stahl GmbH", Definition: syntheticProfile()})
	status, profile := f.call("admin", "POST", "/api/quote-profiles", string(raw))
	if status != 201 {
		t.Fatalf("profile %d %v", status, profile)
	}
	profileID := profile["id"].(string)
	status, q := f.call("admin", "POST", "/api/quotes", fmt.Sprintf(`{"title":"Profiled","customer_org_node_id":%q}`, f.ids["org"]))
	if status != 201 {
		t.Fatalf("create %d %v", status, q)
	}
	quoteID := q["quote_node_id"].(string)
	draftRevision := func() string {
		status, draft := f.call("admin", "GET", "/api/quotes/"+quoteID+"/draft", "")
		if status != 200 {
			t.Fatalf("draft %d %v", status, draft)
		}
		return string(draft["draft_revision"].(json.Number))
	}
	select_ := func(actor, revision, profile string) (int, map[string]any) {
		return f.call(actor, "PUT", "/api/quotes/"+quoteID+"/profile", fmt.Sprintf(`{"expected_draft_revision":%s,"profile_id":%q}`, revision, profile))
	}

	if status, _ := select_("admin", "99", profileID); status != 409 {
		t.Fatalf("stale selection %d", status)
	}
	if status, _ := select_("admin", draftRevision(), "not-a-uuid"); status != 400 {
		t.Fatalf("bad profile id %d", status)
	}
	// A member works on drafts, so a member may choose their look.
	status, draft := select_("member", draftRevision(), profileID)
	if status != 200 {
		t.Fatalf("select %d %v", status, draft)
	}
	doc := draft["document"].(map[string]any)
	chosen, ok := doc["profile"].(map[string]any)
	if !ok || chosen["id"] != profileID || string(chosen["revision"].(json.Number)) != "1" {
		t.Fatalf("draft profile %v", doc["profile"])
	}
	status, draft = select_("admin", draftRevision(), "")
	if status != 200 {
		t.Fatalf("clear %d %v", status, draft)
	}
	if _, still := draft["document"].(map[string]any)["profile"]; still {
		t.Fatalf("profile kept after clearing: %v", draft["document"].(map[string]any)["profile"])
	}
	// Archived profiles are not offered to drafts any more.
	if status, body := f.call("admin", "DELETE", "/api/quote-profiles/"+profileID, ""); status != 204 {
		t.Fatalf("archive %d %v", status, body)
	}
	if status, _ := select_("admin", draftRevision(), profileID); status != 409 {
		t.Fatalf("archived profile selected %d", status)
	}
}
