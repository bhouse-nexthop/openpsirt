package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAFlawInOurOwnProductIsRecordedAndReadBackLikeAnyOther(t *testing.T) {
	// The point of doing this before the reports and the channels: from the
	// moment it is recorded it is an ordinary finding. It appears in the list,
	// it can be assigned, it can be decided — with one difference, which is
	// that nobody outside has been told about it.
	twoReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		const at = "/v1/products/mine/streams/master/variants/broadcom/findings"

		// A reader cannot, and neither can somebody who may only argue about
		// known issues in shipped components.
		body := `{"summary":"The management socket answers before anyone authenticated.",` +
			`"severity":"critical"}`
		for _, who := range []string{"reader", "triager"} {
			if got := asPerson(t, r, who, http.MethodPost, at, body); got.Code < 400 {
				t.Errorf("%s recorded an undisclosed flaw: %d", who, got.Code)
			}
		}

		got := asPerson(t, r, "private-triage", http.MethodPost, at, body)
		if got.Code != http.StatusCreated {
			t.Fatalf("recording answered %d: %s", got.Code, got.Body.String())
		}
		var recorded struct {
			Identifier string `json:"identifier"`
			Component  string `json:"component"`
			Visibility string `json:"visibility"`
			DueAt      string `json:"due_at"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &recorded); err != nil {
			t.Fatalf("decode: %v (%s)", err, got.Body.String())
		}
		if recorded.Identifier != "MINE-2026-0001" {
			t.Errorf("filed under %q, want the product's own first identifier",
				recorded.Identifier)
		}
		if recorded.Visibility != "private" {
			t.Errorf("a flaw nobody announced was recorded as %q", recorded.Visibility)
		}
		if recorded.DueAt == "" {
			t.Error("it carries no deadline, so it is on nobody's clock")
		}

		// And it reads back only for somebody who may see undisclosed work.
		var mine struct {
			Items []struct {
				Vulnerability string `json:"vulnerability"`
			} `json:"items"`
			Total int `json:"total"`
		}
		read(t, r, "private-triage", at, &mine)
		var listed bool
		for _, item := range mine.Items {
			if item.Vulnerability == recorded.Identifier {
				listed = true
			}
		}
		if !listed {
			t.Errorf("what was just recorded is not in the list its author can see: %+v",
				mine.Items)
		}

		var theirs struct {
			Items []struct {
				Vulnerability string `json:"vulnerability"`
			} `json:"items"`
		}
		read(t, r, "triager", at, &theirs)
		for _, item := range theirs.Items {
			if item.Vulnerability == recorded.Identifier {
				t.Errorf("an undisclosed finding is listed for somebody who may not see one")
			}
		}
	})
}

func TestWhatIsApproachingDisclosureIsAnsweredOnlyToWhoMaySeeIt(t *testing.T) {
	// Every row is a finding nobody has announced, so the list is a disclosure
	// in its own right. A product somebody may not read undisclosed work in
	// contributes nothing to it — not even a count, because a count says as
	// much as a row.
	twoReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		const at = "/v1/products/mine/streams/master/variants/broadcom/findings"
		got := asPerson(t, r, "private-triage", http.MethodPost, at,
			`{"summary":"Not announced anywhere.","severity":"critical"}`)
		if got.Code != http.StatusCreated {
			t.Fatalf("recording answered %d: %s", got.Code, got.Body.String())
		}

		var listed struct {
			Items []struct {
				Vulnerability string `json:"vulnerability"`
				DiscloseAt    string `json:"disclose_at"`
				Passed        bool   `json:"passed"`
			} `json:"items"`
		}
		read(t, r, "private-triage", "/v1/disclosing?within=365", &listed)
		if len(listed.Items) != 1 {
			t.Fatalf("%d findings are approaching disclosure for somebody who may see them",
				len(listed.Items))
		}
		if listed.Items[0].DiscloseAt == "" {
			t.Error("the row does not say when the embargo ends")
		}
		if listed.Items[0].Passed {
			t.Error("an embargo ninety days out reads as passed")
		}

		for _, who := range []string{"reader", "triager"} {
			var theirs struct {
				Items []struct {
					Vulnerability string `json:"vulnerability"`
				} `json:"items"`
			}
			read(t, r, who, "/v1/disclosing?within=365", &theirs)
			if len(theirs.Items) != 0 {
				t.Errorf("%s was shown %d undisclosed findings", who, len(theirs.Items))
			}
		}
	})
}
