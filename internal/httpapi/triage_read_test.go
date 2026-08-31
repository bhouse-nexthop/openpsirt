package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
)

// scanned puts a build behind the handler: one component under the product,
// with one issue reported against it. Reading what has been decided is only
// testable against something that was found.
func (r *reach) scanned(t *testing.T) (place string) {
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
		TargetID: target.ID, ContentHash: "read-test", BuiltAt: time.Now().UTC(),
		ParserVersion: "test",
	})
	if err != nil || outcome != ingest.Accept {
		t.Fatalf("record scan: %v %v", outcome, err)
	}

	product := graph.Described{Purl: "pkg:deb/debian/mine@1.0", Name: "mine", Version: "1.0"}
	library := graph.Described{
		Purl: "pkg:deb/debian/libnl-3-200@3.7.0", Name: "libnl-3-200", Version: "3.7.0",
	}
	if _, err := graph.NewStore(r.db.DB).Apply(ctx, target.ID, scan.ID, graph.Snapshot{
		Root:         product,
		Components:   []graph.Described{library},
		Dependencies: []graph.Dependency{{Parent: product, Child: library}},
	}); err != nil {
		t.Fatal(err)
	}

	findings := finding.NewStore(r.db.DB)
	run, err := findings.Begin(ctx, finding.Run{
		TargetID: target.ID, Scanner: "grype", ScannerVersion: "0.112.0",
		DatabaseVersion: "2026-08-28", RanHere: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := findings.Apply(ctx, target.ID, run.ID, []finding.Reported{{
		Issue:     finding.Named{Identifier: "CVE-2026-9999", Severity: "high"},
		Component: library,
		FixState:  finding.FixedUpstream, FixedIn: "3.9.0",
	}}); err != nil {
		t.Fatal(err)
	}

	// Under the product itself, the component stands alone.
	return finding.PlaceIdentity("libnl-3-200", "")
}

// scannedWithEvidence is a build whose one finding carries everything a report
// can carry, so a test can ask whether any of it survives the trip.
func (r *reach) scannedWithEvidence(t *testing.T) {
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
		TargetID: target.ID, ContentHash: "evidence-test", BuiltAt: time.Now().UTC(),
		ParserVersion: "test",
	})
	if err != nil || outcome != ingest.Accept {
		t.Fatalf("record scan: %v %v", outcome, err)
	}

	product := graph.Described{Purl: "pkg:deb/debian/mine@1.0", Name: "mine", Version: "1.0"}
	consumer := graph.Described{
		Purl: "pkg:deb/debian/libswsscommon@1.0.0", Name: "libswsscommon", Version: "1.0.0",
	}
	library := graph.Described{
		Purl: "pkg:deb/debian/libnl-3-200@3.7.0", Name: "libnl-3-200", Version: "3.7.0",
	}
	if _, err := graph.NewStore(r.db.DB).Apply(ctx, target.ID, scan.ID, graph.Snapshot{
		Root:       product,
		Components: []graph.Described{consumer, library},
		Dependencies: []graph.Dependency{
			{Parent: product, Child: consumer}, {Parent: consumer, Child: library},
		},
	}); err != nil {
		t.Fatal(err)
	}

	findings := finding.NewStore(r.db.DB)
	run, err := findings.Begin(ctx, finding.Run{
		TargetID: target.ID, Scanner: "grype", ScannerVersion: "0.112.0",
		DatabaseVersion: "2026-08-28", RanHere: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := findings.Apply(ctx, target.ID, run.ID, []finding.Reported{{
		Issue: finding.Named{
			Identifier:  "CVE-2026-9999",
			Severity:    "high",
			Description: "A crafted attribute length causes a read past the end of the buffer.",
			Advisory:    "https://nvd.nist.gov/vuln/detail/CVE-2026-9999",
			References: []finding.Reference{
				{URL: "https://github.com/thom311/libnl/commit/abc123", Kind: finding.Patch},
				{URL: "https://example.org/write-up", Kind: finding.AdvisoryRef},
			},
			Exploited:  true,
			Likelihood: 0.86,
			Score:      8.1,
			Vector:     "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:H",
			Weaknesses: []string{"CWE-125"},
		},
		Component: library,
		FixState:  finding.FixedUpstream, FixedIn: "3.9.0",
	}}); err != nil {
		t.Fatal(err)
	}
}

// decided proposes a claim through the API and returns its identifier.
func (r *reach) decided(t *testing.T, place string) int64 {
	t.Helper()
	path := fmt.Sprintf("/v1/products/mine/streams/master/variants/broadcom"+
		"/findings/CVE-2026-9999/places/%s/decision", place)
	got := asPerson(t, r, "triager", http.MethodPost, path,
		`{"outcome":"not-applicable","justification":"vulnerable_code_not_in_execute_path",`+
			`"reasoning":"The parser is never reached: we only call the encoder."}`)
	if got.Code != http.StatusCreated {
		t.Fatalf("proposing answered %d: %s", got.Code, got.Body.String())
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, got.Body.String())
	}
	return out.ID
}

func TestEverythingWrittenAboutADecisionCanBeReadBack(t *testing.T) {
	// The gap this closes. A tool that lets somebody argue, agree and annotate
	// and then offers no way to see what any of it produced sends every reader
	// to the review queue — which by definition no longer holds what was
	// agreed to.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)

		if got := asPerson(t, r, "triager", http.MethodPost,
			fmt.Sprintf("/v1/decisions/%d/comments", id),
			`{"body":"Re-checked against 3.7.0; still true."}`); got.Code != http.StatusCreated {
			t.Fatalf("commenting answered %d: %s", got.Code, got.Body.String())
		}
		if got := asPerson(t, r, "reviewer", http.MethodPost,
			fmt.Sprintf("/v1/decisions/%d/approval", id), `{}`); got.Code != http.StatusNoContent {
			t.Fatalf("approving answered %d: %s", got.Code, got.Body.String())
		}

		// The decision itself, saying what it is about rather than by number.
		var detail struct {
			Decision struct {
				Outcome string `json:"outcome"`
				State   string `json:"state"`
			} `json:"decision"`
			Place struct {
				Product       string `json:"product"`
				Vulnerability string `json:"vulnerability"`
				Place         string `json:"place"`
			} `json:"place"`
			Reasoning  string `json:"reasoning"`
			ProposedBy string `json:"proposed_by"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d", id), &detail)
		if detail.Decision.State != "approved" {
			t.Errorf("the decision reads as %q after being approved", detail.Decision.State)
		}
		// "Mine" rather than "mine": what comes back is the spelling somebody
		// declared, not the normalized form we match on.
		if detail.Place.Product != "Mine" || detail.Place.Vulnerability != "CVE-2026-9999" {
			t.Errorf("the decision does not say what it is about: %+v", detail.Place)
		}
		if detail.Reasoning == "" {
			t.Error("the decision came back with no reasoning to read")
		}
		if detail.ProposedBy != "triager" {
			t.Errorf("proposed by %q, want the person who made the claim", detail.ProposedBy)
		}

		// The reasoning, the agreement, and the discussion, each on its own.
		var revisions struct {
			Items []struct {
				ID        int64  `json:"id"`
				Body      string `json:"body"`
				WrittenBy string `json:"written_by"`
			} `json:"items"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d/revisions", id), &revisions)
		if len(revisions.Items) != 1 || revisions.Items[0].WrittenBy != "triager" {
			t.Errorf("the reasoning history reads as %+v", revisions.Items)
		}

		var approvals struct {
			Items []struct {
				RevisionID int64  `json:"revision_id"`
				ApprovedBy string `json:"approved_by"`
			} `json:"items"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d/approvals", id), &approvals)
		if len(approvals.Items) != 1 || approvals.Items[0].ApprovedBy != "reviewer" {
			t.Fatalf("who agreed reads as %+v", approvals.Items)
		}
		// The agreement names the words that were agreed to, which is the
		// whole point of keeping revisions.
		if approvals.Items[0].RevisionID != revisions.Items[0].ID {
			t.Error("the approval does not name a revision anybody can read")
		}

		var comments struct {
			Items []struct {
				Body      string `json:"body"`
				WrittenBy string `json:"written_by"`
			} `json:"items"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d/comments", id), &comments)
		if len(comments.Items) != 1 || comments.Items[0].WrittenBy != "triager" {
			t.Errorf("the discussion reads as %+v", comments.Items)
		}
	})
}

func TestWhatWasDismissedCanBeListed(t *testing.T) {
	// The question somebody auditing asks: what have we decided not to fix,
	// and on what grounds. Without it the only list of decisions is the review
	// queue, which holds exactly the ones nobody has agreed to yet.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		r.decided(t, place)

		var listed struct {
			Items []struct {
				Decision struct {
					Outcome string `json:"outcome"`
				} `json:"decision"`
				Reasoning string `json:"reasoning"`
			} `json:"items"`
			Total int `json:"total"`
		}
		read(t, r, "triager", "/v1/decisions?outcome=not-applicable", &listed)
		if listed.Total != 1 || len(listed.Items) != 1 {
			t.Fatalf("%d dismissals listed, want 1", listed.Total)
		}
		if listed.Items[0].Reasoning == "" {
			t.Error("a dismissal listed without the reason it was dismissed for")
		}

		// A filter that matches nothing says so rather than falling back to
		// everything, which is how a filtered list becomes dangerous.
		var none struct {
			Total int `json:"total"`
		}
		read(t, r, "triager", "/v1/decisions?outcome=wont-fix", &none)
		if none.Total != 0 {
			t.Errorf("filtering by an outcome nothing has returned %d", none.Total)
		}
	})
}

func TestReadingADecisionNeedsTheRightToTakePartInIt(t *testing.T) {
	// Every read is narrowed the same way the writes are. A decision somebody
	// may not reach answers as one that is not there, so guessing identifiers
	// says nothing.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)

		// Holding the approver capability and nothing to read it against
		// reaches nothing, which is what makes a capability a capability.
		if got := asPerson(t, r, "approver", http.MethodPost,
			fmt.Sprintf("/v1/decisions/%d/approval", id), `{}`); got.Code != http.StatusNotFound {
			t.Errorf("an approver with no visibility approved: %d", got.Code)
		}

		for _, path := range []string{
			fmt.Sprintf("/v1/decisions/%d", id),
			fmt.Sprintf("/v1/decisions/%d/revisions", id),
			fmt.Sprintf("/v1/decisions/%d/approvals", id),
			fmt.Sprintf("/v1/decisions/%d/comments", id),
		} {
			got := asPerson(t, r, "reader", http.MethodGet, path, "")
			if got.Code != http.StatusNotFound {
				t.Errorf("%s answered %d to somebody who may only read the product, want 404",
					path, got.Code)
			}
		}

		// And the list is empty rather than refused: they may ask, and the
		// answer is that they can reach none of it.
		var listed struct {
			Total int `json:"total"`
		}
		read(t, r, "reader", "/v1/decisions", &listed)
		if listed.Total != 0 {
			t.Errorf("somebody who may only read was told %d decisions exist", listed.Total)
		}
	})
}

func TestWhatAppliesToAFindingIsReadableWithItsHistory(t *testing.T) {
	// Somebody deciding needs to know what was decided here before. Making
	// them start from a blank page, having thrown away what was written last
	// time, is how a tool teaches people to stop writing reasoning at all.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)
		path := fmt.Sprintf("/v1/products/mine/streams/master/variants/broadcom"+
			"/findings/CVE-2026-9999/places/%s/decision", place)

		var before struct {
			Standing   *struct{} `json:"standing"`
			Previously []struct {
				Decision struct {
					State string `json:"state"`
				} `json:"decision"`
			} `json:"previously"`
		}
		read(t, r, "triager", path, &before)
		// Nobody has agreed to it yet, so it suppresses nothing — but it is
		// there to be read.
		if before.Standing != nil {
			t.Error("a claim nobody agreed to reads as standing")
		}
		if len(before.Previously) != 1 {
			t.Fatalf("the history reads as %+v", before.Previously)
		}

		if got := asPerson(t, r, "reviewer", http.MethodPost,
			fmt.Sprintf("/v1/decisions/%d/approval", id), `{}`); got.Code != http.StatusNoContent {
			t.Fatalf("approving answered %d: %s", got.Code, got.Body.String())
		}

		var after struct {
			Standing *struct {
				Decision struct {
					Outcome string `json:"outcome"`
					State   string `json:"state"`
				} `json:"decision"`
				Reasoning string `json:"reasoning"`
			} `json:"standing"`
		}
		read(t, r, "triager", path, &after)
		if after.Standing == nil {
			t.Fatal("an agreed decision does not read as standing where it was made")
		}
		if after.Standing.Decision.Outcome != "not-applicable" || after.Standing.Reasoning == "" {
			t.Errorf("what stands here reads as %+v", after.Standing)
		}
	})
}

// read makes a GET as somebody and decodes what came back.
func read(t *testing.T, r *reach, who, path string, into any) {
	t.Helper()
	got := asPerson(t, r, who, http.MethodGet, path, "")
	if got.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d: %s", path, got.Code, got.Body.String())
	}
	if err := json.Unmarshal(got.Body.Bytes(), into); err != nil {
		t.Fatalf("decode %s: %v (%s)", path, err, got.Body.String())
	}
}

func TestHTMLIsOfferedOnRequestAndNeverByDefault(t *testing.T) {
	// Markdown is what every consumer gets: it is what an integrating
	// application can most easily lay out, and it reads as plain text as it
	// stands. HTML assumes a browser, which in an API-first tool most callers
	// are not — so a caller that wants it says so.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)

		var plain struct {
			Reasoning     string `json:"reasoning"`
			ReasoningHTML string `json:"reasoning_html"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d", id), &plain)
		if plain.Reasoning == "" {
			t.Error("the default representation carries no text at all")
		}
		if plain.ReasoningHTML != "" {
			t.Errorf("markup came back without being asked for: %q", plain.ReasoningHTML)
		}

		var rendered struct {
			ReasoningHTML string `json:"reasoning_html"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d?html=true", id), &rendered)
		if !strings.Contains(rendered.ReasoningHTML, "<p>") {
			t.Errorf("asking for markup returned %q", rendered.ReasoningHTML)
		}
	})
}

func TestMarkupComesBackSanitizedHoweverItWasStored(t *testing.T) {
	// The sanitizer runs on the way out as well as at submission, because
	// stored text predates rules written since — a control that only ran when
	// the text arrived protects nothing written before it existed. This writes
	// past the submission check to prove the second half runs.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)

		// Straight into the row, the way text stored under an older rule would
		// already be sitting there.
		if _, err := r.db.DB.NewUpdate().
			Table("decision_revision").
			Set("body = ?", "Fine, then <img src=x onerror=alert(1)> and "+
				"[a link](javascript:alert(2)).").
			Where("decision_id = ?", id).
			Exec(t.Context()); err != nil {
			t.Fatal(err)
		}

		var rendered struct {
			ReasoningHTML string `json:"reasoning_html"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d?html=true", id), &rendered)
		for _, forbidden := range []string{"onerror", "javascript:", "<img"} {
			if strings.Contains(rendered.ReasoningHTML, forbidden) {
				t.Errorf("markup carries %q: %s", forbidden, rendered.ReasoningHTML)
			}
		}
	})
}

func TestAFindingCarriesEverythingNeededToActOnIt(t *testing.T) {
	// The measure this is built against: nothing here should send somebody to
	// a search engine. There may be thousands of findings and very few people,
	// so a finding that carries its own evidence and one that does not are the
	// difference between a queue that gets worked and one that does not.
	eachReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)

		var detail struct {
			Vulnerability string   `json:"vulnerability"`
			Severity      string   `json:"severity"`
			Score         float64  `json:"score"`
			Vector        string   `json:"vector"`
			Exploited     bool     `json:"exploited"`
			Likelihood    float64  `json:"likelihood"`
			Weaknesses    []string `json:"weaknesses"`
			Description   string   `json:"description"`
			Advisory      string   `json:"advisory"`
			References    []struct {
				URL  string `json:"url"`
				Kind string `json:"kind"`
			} `json:"references"`
			Component string `json:"component"`
			FixState  string `json:"fix_state"`
			FixedIn   string `json:"fixed_in"`
			Places    []struct {
				Place    string `json:"place"`
				Consumer string `json:"consumer"`
			} `json:"places"`
		}
		read(t, r, "reader", "/v1/products/mine/streams/master/variants/broadcom"+
			"/findings/CVE-2026-9999/components/libnl-3-200", &detail)

		for what, got := range map[string]string{
			"the issue":     detail.Vulnerability,
			"the component": detail.Component,
			"the write-up":  detail.Description,
			"the advisory":  detail.Advisory,
			"the vector":    detail.Vector,
			"the fix state": detail.FixState,
			"the fix":       detail.FixedIn,
		} {
			if got == "" {
				t.Errorf("%s is missing, so somebody has to go and look it up", what)
			}
		}
		if detail.Score == 0 || detail.Likelihood == 0 || !detail.Exploited {
			t.Errorf("what makes this urgent is missing: score=%v likelihood=%v exploited=%v",
				detail.Score, detail.Likelihood, detail.Exploited)
		}
		if len(detail.Weaknesses) == 0 {
			t.Error("what kind of flaw this is was not kept")
		}
		if len(detail.Places) == 0 || detail.Places[0].Place == "" {
			t.Fatalf("the answer does not say where it sits: %+v", detail.Places)
		}

		// The patch comes first. Somebody deciding whether to backport rather
		// than upgrade needs the change itself, and hunting for it among the
		// write-ups is the step that does not happen with a thousand waiting.
		if len(detail.References) < 2 {
			t.Fatalf("the references were not kept: %+v", detail.References)
		}
		if detail.References[0].Kind != "patch" {
			t.Errorf("the first reference is %q, not the change that fixes it",
				detail.References[0].Kind)
		}
	})
}

func TestWorkNobodyOwnsCanBeFoundAndGivenToSomebody(t *testing.T) {
	// Work falling between people is what hides when every screen shows one
	// product: assigned, so not in the shared list; assigned to nobody who is
	// looking, so not in anybody's own.
	eachReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		const at = "/v1/products/mine/streams/master/variants/broadcom" +
			"/findings/CVE-2026-9999/components/libnl-3-200/assignment"

		var waiting struct {
			Items []struct {
				Vulnerability string `json:"vulnerability"`
				Component     string `json:"component"`
				Product       string `json:"product"`
			} `json:"items"`
			Total int `json:"total"`
		}
		read(t, r, "triager", "/v1/unassigned", &waiting)
		if waiting.Total != 1 || len(waiting.Items) != 1 {
			t.Fatalf("%d findings are waiting for an owner, want 1", waiting.Total)
		}
		if waiting.Items[0].Component != "libnl-3-200" || waiting.Items[0].Product != "Mine" {
			t.Errorf("the unassigned row does not say what it is: %+v", waiting.Items[0])
		}

		if got := asPerson(t, r, "triager", http.MethodPut, at,
			`{"person":"reader"}`); got.Code != http.StatusNoContent {
			t.Fatalf("assigning answered %d: %s", got.Code, got.Body.String())
		}

		read(t, r, "triager", "/v1/unassigned", &waiting)
		if waiting.Total != 0 {
			t.Errorf("%d findings still have no owner after being assigned", waiting.Total)
		}

		var holdings struct {
			Items []struct {
				Person string `json:"person"`
				Open   int    `json:"open"`
			} `json:"items"`
		}
		read(t, r, "triager", "/v1/assignments", &holdings)
		if len(holdings.Items) != 1 || holdings.Items[0].Person != "reader" {
			t.Fatalf("who is holding what reads as %+v", holdings.Items)
		}

		// Handing it back is the same action, not a different one.
		if got := asPerson(t, r, "triager", http.MethodPut, at,
			`{"person":""}`); got.Code != http.StatusNoContent {
			t.Fatalf("handing it back answered %d: %s", got.Code, got.Body.String())
		}
		read(t, r, "triager", "/v1/unassigned", &waiting)
		if waiting.Total != 1 {
			t.Errorf("handing it back left %d waiting for an owner", waiting.Total)
		}
	})
}

func TestOnlyAnAdministratorMovesSomebodyElsesWork(t *testing.T) {
	// A person hands back their own by assigning it to nobody. Moving what
	// somebody else was given is an administrative act, and it is the one that
	// matters when they have gone.
	eachReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)
		const at = "/v1/products/mine/streams/master/variants/broadcom" +
			"/findings/CVE-2026-9999/components/libnl-3-200/assignment"
		if got := asPerson(t, r, "triager", http.MethodPut, at,
			`{"person":"reader"}`); got.Code != http.StatusNoContent {
			t.Fatal(got.Body.String())
		}

		release := "/v1/people/reader/assignments/release"
		if got := asPerson(t, r, "triager", http.MethodPost, release, `{}`); got.Code < 400 {
			t.Errorf("a triager released somebody else's work: %d", got.Code)
		}

		got := asPerson(t, r, "admin", http.MethodPost, release, `{}`)
		if got.Code != http.StatusOK {
			t.Fatalf("an administrator releasing work answered %d: %s", got.Code, got.Body.String())
		}
		var moved struct {
			Moved int64 `json:"moved"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &moved); err != nil {
			t.Fatal(err)
		}
		if moved.Moved == 0 {
			t.Error("releasing an absent person's work moved nothing")
		}
	})
}

func TestHowFarADecisionWouldReachComesBackInThreeParts(t *testing.T) {
	// Presenting it as one number is what turns a considered judgment into a
	// reflex, and it is how a decision comes to reach builds the person making
	// it never knew about. The first two parts are consequences of the
	// matching rules and are not choices; only the third is.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		var reached struct {
			Here      int `json:"here"`
			Automatic []struct {
				Stream string `json:"stream"`
			} `json:"automatic"`
			Differing []struct {
				Stream  string `json:"stream"`
				Version string `json:"version"`
			} `json:"differing"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/products/mine/streams/master/variants/broadcom"+
			"/findings/CVE-2026-9999/places/%s/reach", place), &reached)

		if reached.Here != 1 {
			t.Errorf("the judgment covers %d places here, want 1", reached.Here)
		}
		// One build in this deployment, so nothing else to reach either way —
		// what matters is that both lists come back rather than being absent.
		if reached.Automatic == nil || reached.Differing == nil {
			t.Errorf("reach came back incomplete: %+v", reached)
		}
	})
}

func TestTheNewEndpointsAnswerRatherThanExist(t *testing.T) {
	// Each of these was added to close a gap the interface work found, and
	// each one is the kind of thing that can be registered, return an empty
	// shape, and look finished. This drives them against a build that has
	// something in it.
	eachReach(t, func(t *testing.T, r *reach) {
		r.scannedWithEvidence(t)

		// Walking the graph, one step at a time.
		var top struct {
			Items []struct {
				Component string `json:"component"`
				Findings  int    `json:"findings"`
				Children  int    `json:"children"`
			} `json:"items"`
		}
		read(t, r, "reader", "/v1/products/mine/streams/master/variants/broadcom/components", &top)
		if len(top.Items) == 0 {
			t.Fatal("the build reports pulling nothing in")
		}
		var around struct {
			Above []struct {
				Component string `json:"component"`
			} `json:"above"`
			Below []struct {
				Component string `json:"component"`
			} `json:"below"`
		}
		read(t, r, "reader", "/v1/products/mine/streams/master/variants/broadcom"+
			"/components/libnl-3-200/around", &around)
		if len(around.Above) == 0 {
			t.Errorf("nothing pulls libnl-3-200 in, which cannot be true: %+v", around)
		}

		// The issues at a component, which is what a bulk claim narrows.
		var issues struct {
			Items []struct {
				Vulnerability string `json:"vulnerability"`
			} `json:"items"`
			Total int `json:"total"`
		}
		read(t, r, "triager", "/v1/products/mine/streams/master/variants/broadcom"+
			"/components/libnl-3-200/issues", &issues)
		if issues.Total != 1 || len(issues.Items) != 1 {
			t.Fatalf("%d issues at the component, want 1", issues.Total)
		}

		// One claim across a named set.
		got := asPerson(t, r, "triager", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom"+
				"/components/libnl-3-200/decisions",
			`{"vulnerabilities":["CVE-2026-9999"],"outcome":"not-applicable",`+
				`"justification":"vulnerable_code_not_in_execute_path",`+
				`"reasoning":"These are in drivers absent from our kernel config."}`)
		if got.Code != http.StatusCreated {
			t.Fatalf("deciding a set together answered %d: %s", got.Code, got.Body.String())
		}
		var recorded struct {
			Recorded int `json:"recorded"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.Recorded != 1 {
			t.Errorf("recorded %d decisions, want one per issue named", recorded.Recorded)
		}

		// The trend, worked out rather than stored.
		var trend struct {
			Items []struct {
				At   string `json:"at"`
				Open int    `json:"open"`
			} `json:"items"`
		}
		read(t, r, "reader", "/v1/trend?weeks=4", &trend)
		if len(trend.Items) != 4 {
			t.Errorf("asked for four weeks and got %d points", len(trend.Items))
		}
		if trend.Items[len(trend.Items)-1].Open == 0 {
			t.Error("the trend ends with nothing open, but something is")
		}

		// Settings, which had no way to be set at all.
		var settings struct {
			Items []struct {
				Name    string `json:"name"`
				Value   string `json:"value"`
				Default bool   `json:"default"`
			} `json:"items"`
		}
		read(t, r, "admin", "/v1/settings", &settings)
		if len(settings.Items) == 0 {
			t.Fatal("no settings are listed")
		}
		if got := asPerson(t, r, "admin", http.MethodPut,
			"/v1/settings/remediation.due.critical",
			`{"value":"nonsense"}`); got.Code != http.StatusUnprocessableEntity {
			t.Errorf("an unreadable duration was accepted: %d", got.Code)
		}
		if got := asPerson(t, r, "admin", http.MethodPut,
			"/v1/settings/remediation.due.critical",
			`{"value":"48h"}`); got.Code != http.StatusNoContent {
			t.Errorf("setting a deadline answered %d: %s", got.Code, got.Body.String())
		}
	})
}
