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

func TestTheAPIReturnsMarkdownAndNeverMarkup(t *testing.T) {
	// One representation, to every consumer. HTML assumes a browser, and in an
	// API-first tool most callers are not one — the adapters already on the
	// books want neither HTML nor markdown, and markdown is the form an
	// integrating application can most easily lay out and re-render.
	//
	// Our own interface renders in the browser, so it needs no server-rendered
	// half either, and a second renderer is the thing that eventually
	// disagrees with the first.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)

		var body map[string]any
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d", id), &body)
		if body["reasoning"] == "" || body["reasoning"] == nil {
			t.Error("the answer carries no text at all")
		}
		for key := range body {
			if strings.HasSuffix(key, "_html") {
				t.Errorf("the answer carries a rendered field %q", key)
			}
		}

		// And there is no longer any way to ask for markup. An old client
		// still sending html=true is answered rather than refused — unknown
		// query parameters are ignored — and what it gets is the source,
		// never a rendered field it might display without sanitizing.
		var asked map[string]any
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d?html=true", id), &asked)
		for key := range asked {
			if strings.HasSuffix(key, "_html") {
				t.Errorf("html=true still produced a rendered field %q", key)
			}
		}
		if asked["reasoning"] != body["reasoning"] {
			t.Error("html=true changed the answer")
		}
	})
}

func TestStoredTextComesBackExactlyAsWritten(t *testing.T) {
	// The counterpart to rendering in the browser: the server hands back the
	// source it holds, unaltered, so whatever renders it is rendering the
	// authoritative form rather than something already transformed.
	//
	// Sanitizing is the renderer's job and is tested where the renderer lives
	// (internal/markdown). What matters here is that nothing between the row
	// and the reader quietly rewrites the text — a reader that trusted a
	// half-cleaned string would be trusting the wrong control.
	eachReach(t, func(t *testing.T, r *reach) {
		place := r.scanned(t)
		id := r.decided(t, place)

		// Straight into the row, the way text stored under an older rule would
		// already be sitting there.
		hostile := "Fine, then <img src=x onerror=alert(1)> and " +
			"[a link](javascript:alert(2))."
		if _, err := r.db.DB.NewUpdate().
			Table("decision_revision").
			Set("body = ?", hostile).
			Where("decision_id = ?", id).
			Exec(t.Context()); err != nil {
			t.Fatal(err)
		}

		var body struct {
			Reasoning string `json:"reasoning"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d", id), &body)
		if body.Reasoning != hostile {
			t.Errorf("the source came back altered:\n got %q\nwant %q", body.Reasoning, hostile)
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
				Severity      string `json:"severity"`
				Places        int    `json:"places"`
				FixedIn       string `json:"fixed_in"`
			} `json:"items"`
			Total int `json:"total"`
		}
		read(t, r, "triager", "/v1/products/mine/streams/master/variants/broadcom"+
			"/components/libnl-3-200/issues", &issues)
		if issues.Total != 1 || len(issues.Items) != 1 {
			t.Fatalf("%d issues at the component, want 1", issues.Total)
		}
		// The list a person picks from carries what they need to pick: how bad
		// it is, how much of the build it sits in, and whether there is
		// anywhere to go.
		if issues.Items[0].Severity == "" || issues.Items[0].FixedIn == "" ||
			issues.Items[0].Places == 0 {
			t.Errorf("the list to choose from says nothing to choose on: %+v", issues.Items[0])
		}

		// One claim across a named set.
		got := asPerson(t, r, "triager", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom"+
				"/components/libnl-3-200/decisions",
			`{"vulnerabilities":["CVE-2026-9999"],"outcome":"not-applicable",`+
				`"justification":"vulnerable_code_not_in_execute_path",`+
				`"selected_by":"searched the reports for \"driver\"",`+
				`"reasoning":"These are in drivers absent from our kernel config."}`)
		if got.Code != http.StatusCreated {
			t.Fatalf("deciding a set together answered %d: %s", got.Code, got.Body.String())
		}
		var recorded struct {
			Recorded int     `json:"recorded"`
			IDs      []int64 `json:"ids"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &recorded); err != nil {
			t.Fatal(err)
		}
		// One per *place*, not one per name. A decision is keyed on a place,
		// so a claim built from one of them would silence one consumer and
		// leave the rest open while reporting that it had covered them.
		var places int
		for _, each := range issues.Items {
			places += each.Places
		}
		if recorded.Recorded != places {
			t.Errorf("recorded %d decisions for %d places, want one each",
				recorded.Recorded, places)
		}

		// And how the set was narrowed is on the record, because "how were
		// these chosen" is the question asked of a bulk judgment later.
		var made struct {
			Decision struct {
				SelectedBy string `json:"selected_by"`
			} `json:"decision"`
		}
		read(t, r, "triager", fmt.Sprintf("/v1/decisions/%d", recorded.IDs[0]), &made)
		if made.Decision.SelectedBy == "" {
			t.Error("a claim recorded as one of many does not say how the set was narrowed")
		}

		// A deferral with nowhere to say until when is refused rather than
		// recorded as a postponement with no end.
		if got := asPerson(t, r, "triager", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom"+
				"/components/libnl-3-200/decisions",
			`{"vulnerabilities":["CVE-2026-9999"],"outcome":"deferred",`+
				`"selected_by":"the same search","reasoning":"Not this release."}`,
		); got.Code != http.StatusUnprocessableEntity {
			t.Errorf("deferring with no date answered %d: %s", got.Code, got.Body.String())
		}

		// A name nobody here knows says which name, so a person who pasted a
		// list can fix the list rather than bisect it.
		if got := asPerson(t, r, "triager", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom"+
				"/components/libnl-3-200/decisions",
			`{"vulnerabilities":["CVE-2026-9999","CVE-1999-0001"],"outcome":"wont-fix",`+
				`"selected_by":"a list somebody pasted","reasoning":"Not worth it."}`,
		); got.Code != http.StatusNotFound ||
			!strings.Contains(got.Body.String(), "CVE-1999-0001") {
			t.Errorf("an unknown issue answered %d without naming it: %s",
				got.Code, got.Body.String())
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

func TestNobodyLearnsWhoHasAnAccountByBeingRefused(t *testing.T) {
	// A name nobody holds and a name somebody holds must come back the same
	// way to anybody not authorized to act on either, or the refusal is a
	// directory of the organization readable by every account.
	eachReach(t, func(t *testing.T, r *reach) {
		r.scanned(t)

		// Handing back somebody's work is administrative. A reader asking
		// about a real account and a made-up one gets one answer.
		real := asPerson(t, r, "reader", http.MethodPost,
			"/v1/people/triager/assignments/release", `{}`)
		invented := asPerson(t, r, "reader", http.MethodPost,
			"/v1/people/nobody-at-all/assignments/release", `{}`)
		if real.Code != invented.Code {
			t.Errorf("releasing a real person's work answered %d and an invented one %d",
				real.Code, invented.Code)
		}
		if real.Code != http.StatusForbidden {
			t.Errorf("a reader releasing somebody's work answered %d, want 403", real.Code)
		}

		// Assigning is the same shape: reading a product is not being able to
		// hand its findings around, so the name is never resolved first.
		at := "/v1/products/mine/streams/master/variants/broadcom" +
			"/findings/CVE-2026-9999/components/libnl-3-200/assignment"
		known := asPerson(t, r, "reader", http.MethodPut, at, `{"person":"triager"}`)
		unknown := asPerson(t, r, "reader", http.MethodPut, at, `{"person":"nobody-at-all"}`)
		if known.Code != unknown.Code || known.Body.String() != unknown.Body.String() {
			t.Errorf("assigning to a real person answered %d %s and to an invented one %d %s",
				known.Code, known.Body.String(), unknown.Code, unknown.Body.String())
		}
	})
}

func TestASettingThatWouldReadAsUnsetIsRefused(t *testing.T) {
	// Every reader treats zero and negative as unset and falls back to the
	// shipped value, so storing one produces a setting that looks set on the
	// administration screen and does nothing at all.
	eachReach(t, func(t *testing.T, r *reach) {
		for _, value := range []string{"0h", "-48h", "nonsense"} {
			if got := asPerson(t, r, "admin", http.MethodPut,
				"/v1/settings/remediation.due.critical",
				`{"value":"`+value+`"}`); got.Code != http.StatusUnprocessableEntity {
				t.Errorf("%q was accepted as a deadline: %d %s",
					value, got.Code, got.Body.String())
			}
		}

		// The one setting that is a count rather than a duration is checked as
		// a count. A duration there would be stored and then ignored.
		cap := "/v1/settings/triage.together-cap"
		if got := asPerson(t, r, "admin", http.MethodPut, cap,
			`{"value":"48h"}`); got.Code != http.StatusUnprocessableEntity {
			t.Errorf("a duration was accepted as a count: %d %s", got.Code, got.Body.String())
		}
		if got := asPerson(t, r, "admin", http.MethodPut, cap,
			`{"value":"0"}`); got.Code != http.StatusUnprocessableEntity {
			t.Errorf("a cap of nothing was accepted: %d %s", got.Code, got.Body.String())
		}
		if got := asPerson(t, r, "admin", http.MethodPut, cap,
			`{"value":"5"}`); got.Code != http.StatusNoContent {
			t.Errorf("setting the cap answered %d: %s", got.Code, got.Body.String())
		}
	})
}

func TestWithdrawingSomebodysLastRoleHandsBackWhatTheyHeld(t *testing.T) {
	// ACC-43. Otherwise their work is in no list at all: assigned, so not in
	// the shared one, and assigned to somebody who can no longer open it.
	eachReach(t, func(t *testing.T, r *reach) {
		r.scanned(t)
		at := "/v1/products/mine/streams/master/variants/broadcom" +
			"/findings/CVE-2026-9999/components/libnl-3-200/assignment"
		if got := asPerson(t, r, "triager", http.MethodPut, at,
			`{"person":"triager"}`); got.Code != http.StatusNoContent {
			t.Fatalf("assigning answered %d: %s", got.Code, got.Body.String())
		}

		var holdings struct {
			Items []struct {
				Person string `json:"person"`
				Open   int    `json:"open"`
			} `json:"items"`
		}
		read(t, r, "admin", "/v1/assignments", &holdings)
		if len(holdings.Items) == 0 {
			t.Fatal("nothing was assigned to begin with")
		}

		got := asPerson(t, r, "admin", http.MethodDelete,
			"/v1/people/triager/roles/mine/public-triage", "")
		if got.Code != http.StatusOK {
			t.Fatalf("withdrawing a role answered %d: %s", got.Code, got.Body.String())
		}
		var withdrawn struct {
			Released int64 `json:"released"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &withdrawn); err != nil {
			t.Fatal(err)
		}
		if withdrawn.Released == 0 {
			t.Error("withdrawing their last role here handed nothing back")
		}

		read(t, r, "admin", "/v1/assignments", &holdings)
		if len(holdings.Items) != 0 {
			t.Errorf("%d people still hold work here after losing their role",
				len(holdings.Items))
		}
	})
}

func TestABulkClaimCoversWhatTheClaimantCanSeeAndNoMore(t *testing.T) {
	// A public triager's judgment covers the places they can read. The
	// undisclosed ones stay open for whoever can read them — which is the
	// ordinary division of work, not a gap. Refusing the whole action because
	// something undisclosed sits at the same component would answer somebody
	// who picked from the list they were shown with a bare "not found".
	eachReach(t, func(t *testing.T, r *reach) {
		r.scanned(t)

		// A second place for the same issue, so the component holds one the
		// public triager may read and one they may not.
		var rows []finding.Finding
		if err := r.db.DB.NewSelect().Model(&rows).Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("the fixture has %d findings, want 1", len(rows))
		}
		second := rows[0]
		second.ID = 0
		// A different identity of the same width. A place identity is a hash
		// stored in a fixed-width column, so appending to one overflows on
		// three of the four engines and is silently accepted on the fourth.
		second.PlaceIdentity = second.PlaceIdentity[:len(second.PlaceIdentity)-4] + "beef"
		if _, err := r.db.DB.NewInsert().Model(&second).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := r.db.DB.NewUpdate().Table("finding").
			Set("visibility = ?", "private").
			Where("id = ?", rows[0].ID).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}

		at := "/v1/products/mine/streams/master/variants/broadcom" +
			"/components/libnl-3-200/decisions"
		body := `{"vulnerabilities":["CVE-2026-9999"],"outcome":"wont-fix",` +
			`"selected_by":"everything at this component","reasoning":"Not worth the churn."}`

		// One of the two places is theirs to argue about, and that is what the
		// claim covers.
		got := asPerson(t, r, "triager", http.MethodPost, at, body)
		if got.Code != http.StatusCreated {
			t.Fatalf("a public triager answered %d: %s", got.Code, got.Body.String())
		}
		var recorded struct {
			Recorded int `json:"recorded"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.Recorded != 1 {
			t.Errorf("a public triager covered %d places, want only the disclosed one",
				recorded.Recorded)
		}

		// And the undisclosed one is still open, waiting for somebody who can
		// see it.
		standing, err := r.db.DB.NewSelect().Table("decision").
			Where("place_identity = ?", rows[0].PlaceIdentity).
			Where("live_key IS NOT NULL").Count(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if standing != 0 {
			t.Errorf("%d claims stand against a place the claimant cannot read", standing)
		}
	})
}

func TestTheCallerIsToldWhatTheyMayDoRatherThanFindingOut(t *testing.T) {
	// A screen has to know whether to offer an action before it draws one.
	// Without this the client either offers everything and lets people walk
	// into a refusal, or re-implements the mapping from roles to capabilities
	// and drifts from the one the server actually enforces.
	eachReach(t, func(t *testing.T, r *reach) {
		type can struct {
			Product   string `json:"product"`
			MaySee    bool   `json:"may_see"`
			SeesAll   bool   `json:"sees_all"`
			MayTriage bool   `json:"may_triage"`
			MayHide   bool   `json:"may_hide"`
			MayAgree  bool   `json:"may_agree"`
		}
		type who struct {
			Identity string `json:"identity"`
			Name     string `json:"name"`
			Admin    bool   `json:"admin"`
			Kind     string `json:"kind"`
			Reach    []can  `json:"reach"`
		}

		var reader who
		read(t, r, "reader", "/v1/session/me", &reader)
		if reader.Identity != "reader" || reader.Kind != "person" {
			t.Errorf("a reader is described as %+v", reader)
		}
		if reader.Name == "" {
			t.Error("nothing to show in a header")
		}
		if reader.Admin {
			t.Error("a reader is reported as an administrator")
		}
		if len(reader.Reach) == 0 {
			t.Fatal("a reader reaches no product at all")
		}
		for _, each := range reader.Reach {
			if !each.MaySee {
				t.Errorf("%s is listed but cannot be seen", each.Product)
			}
			// Reading what is disclosed is not reading what is not, and it is
			// certainly not deciding. This is the assertion that catches a
			// capability widened by accident.
			if each.SeesAll || each.MayTriage || each.MayHide || each.MayAgree {
				t.Errorf("a reader is offered more than reading in %s: %+v", each.Product, each)
			}
		}

		var triager who
		read(t, r, "triager", "/v1/session/me", &triager)
		for _, each := range triager.Reach {
			if !each.MayTriage {
				t.Errorf("a triager may not triage %s", each.Product)
			}
			if each.MayHide {
				t.Errorf("a public triager is offered undisclosed findings in %s", each.Product)
			}
		}

		// An administrator reaches everything, which is the one role not held
		// against a product.
		var admin who
		read(t, r, "admin", "/v1/session/me", &admin)
		if !admin.Admin {
			t.Error("the administrator is not described as one")
		}
		if len(admin.Reach) < len(reader.Reach) {
			t.Errorf("an administrator reaches %d products where a reader reaches %d",
				len(admin.Reach), len(reader.Reach))
		}
	})
}

func TestOnlyPeopleWhoCanAlreadySeeItAreOfferedAsMentions(t *testing.T) {
	// An autocomplete listing everybody teaches somebody to name a colleague
	// who then cannot open what they were called to. On an undisclosed finding
	// it is worse than unhelpful: the mention itself says a finding exists,
	// which is the disclosure the visibility rule is there to prevent.
	eachReach(t, func(t *testing.T, r *reach) {
		type person struct {
			Identity string `json:"identity"`
			Name     string `json:"name"`
		}
		type list struct {
			Items []person `json:"items"`
		}
		has := func(items []person, who string) bool {
			for _, each := range items {
				if each.Identity == who {
					return true
				}
			}
			return false
		}

		// Everybody who can read what has been disclosed.
		var public list
		read(t, r, "triager", "/v1/products/mine/mentionable", &public)
		if !has(public.Items, "reader") {
			t.Errorf("somebody who can read the product is not offered: %+v", public.Items)
		}
		if has(public.Items, "nothing") {
			t.Error("somebody granted nothing here is offered as a mention")
		}

		// And undisclosed findings are a narrower set. Somebody trusted only
		// with what has been disclosed must not be offered for one that has
		// not — naming them would call them to something they cannot open.
		var private list
		read(t, r, "private-triage", "/v1/products/mine/mentionable?visibility=private", &private)
		if has(private.Items, "reader") {
			t.Errorf("somebody who reads only disclosed findings is offered on an undisclosed one: %+v",
				private.Items)
		}
		if len(private.Items) == 0 {
			t.Error("nobody at all may be mentioned on an undisclosed finding")
		}

		// Asking who may be told about an undisclosed finding is itself a
		// question about undisclosed findings, so somebody who cannot read
		// them is answered as though the product were not there.
		if got := asPerson(t, r, "triager", http.MethodGet,
			"/v1/products/mine/mentionable?visibility=private", ""); got.Code != http.StatusNotFound {
			t.Errorf("a public triager asked about undisclosed mentions and got %d: %s",
				got.Code, got.Body.String())
		}
	})
}
