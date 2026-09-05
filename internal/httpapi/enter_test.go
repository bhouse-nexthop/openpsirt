package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
)

func TestAFlawInOurOwnProductIsRecordedAndReadBackLikeAnyOther(t *testing.T) {
	// The point of doing this before the reports and the channels: from the
	// moment it is recorded it is an ordinary finding. It appears in the list,
	// it can be assigned, it can be decided — with one difference, which is
	// that nobody outside has been told about it.
	twoReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		const at = "/v1/products/mine/streams/master/variants/broadcom/findings"
		// Recording belongs to a build; the list it appears in takes the build
		// as a selection (UIX-53).
		const listing = "/v1/products/mine/findings?stream=master&variant=broadcom"

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
		read(t, r, "private-triage", listing, &mine)
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
		read(t, r, "triager", listing, &theirs)
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

func TestMovingADisclosureDateIsRecordedAndGatedTheSameWayADeferralIs(t *testing.T) {
	// A short extension is ordinary triage; past the threshold it needs a
	// second person, and until it has one the date has not moved. The request
	// is on record either way, because what was asked for is part of how long
	// this stayed hidden.
	twoReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		const findings = "/v1/products/mine/streams/master/variants/broadcom/findings"
		got := asPerson(t, r, "private-triage", http.MethodPost, findings,
			`{"summary":"Not announced anywhere.","severity":"high"}`)
		if got.Code != http.StatusCreated {
			t.Fatalf("recording answered %d: %s", got.Code, got.Body.String())
		}
		var recorded struct {
			Identifier string `json:"identifier"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &recorded); err != nil {
			t.Fatal(err)
		}
		at := "/v1/products/mine/issues/" + recorded.Identifier + "/disclosure"

		// A reason is required.
		if got := asPerson(t, r, "private-triage", http.MethodPost, at,
			`{"until":"2030-01-01","reason":""}`); got.Code < 400 {
			t.Errorf("an embargo was extended for no stated reason: %d", got.Code)
		}
		// And somebody who may not see undisclosed work cannot move one.
		if got := asPerson(t, r, "triager", http.MethodPost, at,
			`{"until":"2030-01-01","reason":"Because."}`); got.Code < 400 {
			t.Errorf("somebody holding only public triage moved an embargo: %d", got.Code)
		}

		// Years out, so well past the threshold: it waits.
		got = asPerson(t, r, "private-triage", http.MethodPost, at,
			`{"until":"2030-01-01","reason":"Upstream has not answered."}`)
		if got.Code != http.StatusCreated {
			t.Fatalf("asking answered %d: %s", got.Code, got.Body.String())
		}
		var asked struct {
			ID            int64 `json:"id"`
			NeedsApproval bool  `json:"needs_approval"`
			InForce       bool  `json:"in_force"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &asked); err != nil {
			t.Fatal(err)
		}
		if !asked.NeedsApproval || asked.InForce {
			t.Errorf("a four-year extension stood on one person's say-so: %+v", asked)
		}

		// The person who asked may not agree to it.
		approval := fmt.Sprintf("/v1/disclosure-extensions/%d/approval", asked.ID)
		if got := asPerson(t, r, "private-triage", http.MethodPost, approval, `{}`); got.Code != http.StatusConflict {
			t.Errorf("agreeing to one's own extension answered %d, want 409", got.Code)
		}

		// The history is kept whether or not anybody agreed.
		var history struct {
			Items []struct {
				Reason  string `json:"reason"`
				InForce bool   `json:"in_force"`
			} `json:"items"`
		}
		read(t, r, "private-triage", at, &history)
		if len(history.Items) != 1 || history.Items[0].Reason == "" {
			t.Fatalf("the record of what was asked reads as %+v", history.Items)
		}
		if history.Items[0].InForce {
			t.Error("an extension nobody agreed to is reported as in force")
		}
	})
}

// shipsTwice re-applies the build's graph holding one name at two versions,
// which is the ordinary case rather than a contrived one: a real image ships
// three vendored copies of one library.
func (r *reach) shipsTwice(t *testing.T) {
	t.Helper()
	ctx := t.Context()

	names := catalog.NewStore(r.db.DB)
	located, err := names.Locate(ctx, "mine", "master", "broadcom")
	if err != nil {
		t.Fatal(err)
	}
	target, err := names.TargetFor(ctx, located.StreamID, located.VariantID)
	if err != nil {
		t.Fatal(err)
	}
	scan, outcome, err := ingest.NewStore(r.db.DB).Record(ctx, ingest.Arriving{
		TargetID: target.ID, ContentHash: "two-versions", ParserVersion: "test",
		// Newer than the first, and not ahead of the clock: a build stamped in
		// the future is refused, because accepting one means no later scan is
		// ever newer.
		BuiltAt: time.Now().UTC(),
	})
	if err != nil || outcome != ingest.Accept {
		t.Fatalf("record scan: %v %v", outcome, err)
	}

	product := graph.Described{Purl: "pkg:deb/debian/mine@1.0", Name: "mine", Version: "1.0"}
	consumer := graph.Described{
		Purl: "pkg:deb/debian/libswsscommon@1.0.0", Name: "libswsscommon", Version: "1.0.0",
	}
	older := graph.Described{
		Purl: "pkg:deb/debian/libnl-3-200@3.7.0", Name: "libnl-3-200", Version: "3.7.0",
	}
	newer := graph.Described{
		Purl: "pkg:deb/debian/libnl-3-200@3.9.0", Name: "libnl-3-200", Version: "3.9.0",
	}
	if _, err := graph.NewStore(r.db.DB).Apply(ctx, target.ID, scan.ID, graph.Snapshot{
		Root:       product,
		Components: []graph.Described{consumer, older, newer},
		Dependencies: []graph.Dependency{
			{Parent: product, Child: consumer},
			{Parent: consumer, Child: older},
			{Parent: consumer, Child: newer},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestANameTheBuildHoldsTwiceIsRefusedWithTheChoicesToPickFrom(t *testing.T) {
	// The screen recording a flaw offers the choices back, so what it needs is
	// which versions rather than only that it could not tell. Resolving to one
	// of them would file a flaw against a version nobody named and say nothing.
	twoReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		r.shipsTwice(t)
		const at = "/v1/products/mine/streams/master/variants/broadcom/findings"
		const said = `"summary":"The parser accepts a message it should refuse.","severity":"high"`

		got := asPerson(t, r, "private-triage", http.MethodPost, at,
			`{`+said+`,"component":"libnl-3-200"}`)
		if got.Code != http.StatusConflict {
			t.Fatalf("an ambiguous name answered %d: %s", got.Code, got.Body.String())
		}
		var refused struct {
			Errors []struct {
				Location string            `json:"location"`
				Message  string            `json:"message"`
				Value    map[string]string `json:"value"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &refused); err != nil {
			t.Fatalf("decode: %v (%s)", err, got.Body.String())
		}
		if len(refused.Errors) != 2 {
			t.Fatalf("the refusal offers %d choices, want both: %s", len(refused.Errors),
				got.Body.String())
		}
		// The ecosystem travels with each, because a version alone does not
		// always resolve one — a source repository and the package built from
		// it share both a name and a version.
		for _, choice := range refused.Errors {
			if choice.Value["ecosystem"] == "" {
				t.Errorf("choice %q offers no ecosystem", choice.Message)
			}
		}

		// Named, it is recorded against that one.
		made := asPerson(t, r, "private-triage", http.MethodPost, at,
			`{`+said+`,"component":"libnl-3-200","version":"3.9.0"}`)
		if made.Code != http.StatusCreated {
			t.Fatalf("naming the version answered %d: %s", made.Code, made.Body.String())
		}
	})
}

func TestASummaryOfNothingButSpacesIsTheCallersToFix(t *testing.T) {
	// Whitespace passes a minimum length, so this reaches the store, and the
	// store refusing it used to arrive as a 500 saying something went wrong
	// here. Nothing went wrong here.
	twoReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		got := asPerson(t, r, "private-triage", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom/findings",
			`{"summary":"   ","severity":"high"}`)
		if got.Code != http.StatusUnprocessableEntity {
			t.Errorf("a summary of spaces answered %d, want it named as the caller's to fix: %s",
				got.Code, got.Body.String())
		}
	})
}
