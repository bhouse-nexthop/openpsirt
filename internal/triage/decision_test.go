package triage_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// fixture is a product, an issue and two people, which is the least a decision
// needs: one to make a claim and one to agree with it.
type fixture struct {
	store   *triage.Store
	product int64
	issue   int64
	proposer,
	approver int64
}

// at is a place to decide about, defaulting to versions that make the
// straightforward case straightforward.
func (f *fixture) at() triage.Place {
	return triage.Place{
		ProductID: f.product, VulnerabilityID: f.issue,
		PlaceIdentity:     "place-of-libfoo-under-libbar",
		ComponentUpstream: "1.2.3", ConsumerUpstream: "4.5.6",
	}
}

// claims a not-applicable decision, which is the shape everything else builds on.
func (f *fixture) claims(t *testing.T, at triage.Place) *triage.Decision {
	t.Helper()
	decision, err := f.store.Propose(t.Context(), triage.Proposal{
		Place: at, Outcome: triage.NotApplicable,
		Justification: triage.CodeNotInExecutePath,
		Reasoning:     "The parser is never reached: we only call the encoder.",
		By:            f.proposer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func each(t *testing.T, fn func(t *testing.T, f *fixture)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		product, err := catalog.NewStore(db.DB).DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		interned, err := finding.NewVulnerabilities(db.DB).Intern(ctx, []finding.Named{
			{Identifier: "CVE-2026-1", Severity: "high"},
		})
		if err != nil {
			t.Fatal(err)
		}
		issue := interned["CVE-2026-1"]
		rights := access.NewStore(db.DB)
		one, err := rights.Ensure(ctx, "proposer", "", true)
		if err != nil {
			t.Fatal(err)
		}
		two, err := rights.Ensure(ctx, "approver", "", true)
		if err != nil {
			t.Fatal(err)
		}

		fn(t, &fixture{
			store: triage.NewStore(db.DB), product: product.ID, issue: issue,
			proposer: one.ID, approver: two.ID,
		})
	})
}

func TestADecisionStandsAgainstThePlaceItWasMadeAbout(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		claimed := f.claims(t, f.at())

		standing, err := f.store.Applying(t.Context(), f.at())
		if err != nil {
			t.Fatal(err)
		}
		if standing == nil || standing.ID != claimed.ID {
			t.Fatalf("the decision just made does not apply where it was made: %+v", standing)
		}
		if standing.RevisionID == nil {
			t.Error("a decision was recorded with no reasoning to read")
		}
	})
}

func TestAnUpstreamVersionMovingLapsesADecision(t *testing.T) {
	// Expiry is not a mechanism. The decision was stored under the versions it
	// was a claim about, and a place asks under the versions it has now — so
	// when the code moves, the keys stop matching and the claim stops
	// standing. Nothing sweeps and nothing runs on a timer.
	each(t, func(t *testing.T, f *fixture) {
		f.claims(t, f.at())

		for _, moved := range []struct {
			what  string
			place triage.Place
		}{
			{"the component's upstream version", func() triage.Place {
				p := f.at()
				p.ComponentUpstream = "1.2.4"
				return p
			}()},
			{"the consumer's upstream version", func() triage.Place {
				p := f.at()
				p.ConsumerUpstream = "4.5.7"
				return p
			}()},
			{"the place itself", func() triage.Place {
				p := f.at()
				p.PlaceIdentity = "somewhere-else"
				return p
			}()},
			{"the issue", func() triage.Place {
				p := f.at()
				p.VulnerabilityID = f.issue + 1000
				return p
			}()},
		} {
			standing, err := f.store.Applying(t.Context(), moved.place)
			if err != nil {
				t.Fatal(err)
			}
			if standing != nil {
				t.Errorf("the decision still stood after %s changed", moved.what)
			}
		}
	})
}

func TestARebuildDoesNotLapseADecision(t *testing.T) {
	// The reason only upstream versions are compared. A shipped package is
	// rebuilt constantly and carries a version of its own that moves each
	// time — and a rebuild is not somebody's reasoning becoming wrong. If the
	// shipped version were compared, every decision would lapse nightly and
	// the tool would be unusable by the second week.
	each(t, func(t *testing.T, f *fixture) {
		f.claims(t, f.at())

		// Same upstream, and everything about how it was packaged has moved.
		// The place is derived from names, so a repackage does not touch it.
		rebuilt := f.at()
		standing, err := f.store.Applying(t.Context(), rebuilt)
		if err != nil {
			t.Fatal(err)
		}
		if standing == nil {
			t.Error("a decision lapsed on a rebuild that changed no upstream version")
		}
	})
}

func TestADecisionIsFoundAgainAfterItLapses(t *testing.T) {
	// Making somebody re-decide from a blank page, having thrown away what was
	// written last time, is how a tool teaches people to stop writing
	// reasoning at all.
	each(t, func(t *testing.T, f *fixture) {
		f.claims(t, f.at())

		moved := f.at()
		moved.ComponentUpstream = "1.2.4"
		if standing, _ := f.store.Applying(t.Context(), moved); standing != nil {
			t.Fatal("the decision should not stand at the moved version")
		}

		previous, err := f.store.PreviouslyAt(t.Context(), moved)
		if err != nil {
			t.Fatal(err)
		}
		if len(previous) != 1 {
			t.Fatalf("found %d earlier decisions about this place, want 1", len(previous))
		}
		if previous[0].ComponentUpstreamVersion == nil || *previous[0].ComponentUpstreamVersion != "1.2.3" {
			t.Error("what came back does not say which version it was a claim about")
		}
	})
}

func TestNobodyApprovesTheirOwnClaim(t *testing.T) {
	// No override. A deployment with one person cannot approve anything, which
	// is the control working rather than a gap in it.
	each(t, func(t *testing.T, f *fixture) {
		claimed := f.claims(t, f.at())

		if err := f.store.Approve(t.Context(), claimed.ID, f.proposer, ""); !errors.Is(err, triage.ErrSamePerson) {
			t.Errorf("somebody approved their own claim: %v", err)
		}
		if err := f.store.Approve(t.Context(), claimed.ID, f.approver, ""); err != nil {
			t.Errorf("a second person could not approve: %v", err)
		}
	})
}

func TestRevisingTheReasoningTakesBackTheApproval(t *testing.T) {
	// The control, and the way it fails silently if this is wrong. A second
	// person agreed to particular words; different words are a claim nobody
	// has agreed to, and an approval carrying over would leave the record
	// saying two people had read something only one of them had.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.claims(t, f.at())
		if err := f.store.Approve(ctx, claimed.ID, f.approver, ""); err != nil {
			t.Fatal(err)
		}

		if _, err := f.store.Revise(ctx, claimed.ID, f.proposer,
			"Actually the parser is reached, but only from a path we control."); err != nil {
			t.Fatal(err)
		}

		standing, err := f.store.Applying(ctx, f.at())
		if err != nil {
			t.Fatal(err)
		}
		if standing == nil || standing.State != triage.Proposed {
			t.Fatalf("after revising, the decision reads as %v", standing)
		}

		approvals, err := f.store.Approvals(ctx, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(approvals) != 1 || approvals[0].WithdrawnAt == nil {
			t.Errorf("the approval on the old words is still standing: %+v", approvals)
		}

		// And what was agreed to is still readable, which is the point of
		// keeping revisions rather than overwriting them.
		revisions, err := f.store.Revisions(ctx, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(revisions) != 2 {
			t.Fatalf("kept %d revisions, want 2", len(revisions))
		}
		if revisions[0].ID != approvals[0].RevisionID {
			t.Error("the approval does not name the words that were approved")
		}
	})
}

func TestADeferralStopsStandingOnItsDate(t *testing.T) {
	// A different claim, so a different mechanism: a version bump does not
	// change a judgment about priority, and a calendar does not change one
	// about applicability.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		soon := time.Now().UTC().Add(24 * time.Hour)
		if _, err := f.store.Propose(ctx, triage.Proposal{
			Place: f.at(), Outcome: triage.Deferred, DeferredUntil: &soon,
			Reasoning: "Not this sprint.", By: f.proposer,
		}); err != nil {
			t.Fatal(err)
		}
		if standing, _ := f.store.Applying(ctx, f.at()); standing == nil {
			t.Error("a deferral did not stand before its date")
		}

		past := time.Now().UTC().Add(-time.Hour)
		lapsedPlace := f.at()
		lapsedPlace.PlaceIdentity = "another-place"
		if _, err := f.store.Propose(ctx, triage.Proposal{
			Place: lapsedPlace, Outcome: triage.Deferred, DeferredUntil: &past,
			Reasoning: "Was not that sprint either.", By: f.proposer,
		}); err != nil {
			t.Fatal(err)
		}
		if standing, _ := f.store.Applying(ctx, lapsedPlace); standing != nil {
			t.Error("a deferral still stood after its date had passed")
		}
	})
}

func TestAClaimHasToSayEnoughToBeAgreedWith(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		for _, c := range []struct {
			what string
			p    triage.Proposal
		}{
			{"no reasoning", triage.Proposal{Place: f.at(), Outcome: triage.WontFix, By: f.proposer}},
			{"no outcome", triage.Proposal{Place: f.at(), Reasoning: "x", By: f.proposer}},
			{"nobody making it", triage.Proposal{Place: f.at(), Outcome: triage.WontFix, Reasoning: "x"}},
			{"not-applicable with no reason", triage.Proposal{
				Place: f.at(), Outcome: triage.NotApplicable, Reasoning: "x", By: f.proposer}},
			{"not-applicable with an unrecognized reason", triage.Proposal{
				Place: f.at(), Outcome: triage.NotApplicable, Justification: "because-i-say-so",
				Reasoning: "x", By: f.proposer}},
			{"a deferral with no date to return on", triage.Proposal{
				Place: f.at(), Outcome: triage.Deferred, Reasoning: "x", By: f.proposer}},
			{"a reason on a claim that is not about applicability", triage.Proposal{
				Place: f.at(), Outcome: triage.WontFix, Justification: triage.CodeNotPresent,
				Reasoning: "x", By: f.proposer}},
		} {
			if _, err := f.store.Propose(t.Context(), c.p); err == nil {
				t.Errorf("a claim with %s was recorded", c.what)
			}
		}
	})
}

func TestABulkApprovalIsUndoneAsABatch(t *testing.T) {
	// A reviewer may agree to a long selection in one action, so undoing has
	// to be available at the same size. Hunting for what a bulk approval
	// touched, one row at a time, is not an undo anybody will use.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		const batch = "one-afternoon"
		var made []int64
		for _, place := range []string{"a", "b", "c"} {
			at := f.at()
			at.PlaceIdentity = place
			decision := f.claims(t, at)
			made = append(made, decision.ID)
			if err := f.store.Approve(ctx, decision.ID, f.approver, batch); err != nil {
				t.Fatal(err)
			}
		}

		undone, err := f.store.UndoBatch(ctx, batch)
		if err != nil {
			t.Fatal(err)
		}
		if undone != int64(len(made)) {
			t.Errorf("undid %d of %d", undone, len(made))
		}
		for _, id := range made {
			approvals, err := f.store.Approvals(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if len(approvals) != 1 || approvals[0].WithdrawnAt == nil {
				t.Errorf("decision %d is still approved", id)
			}
		}
		// The claims still stand; it is the agreement that was taken back.
		at := f.at()
		at.PlaceIdentity = "a"
		if standing, _ := f.store.Applying(ctx, at); standing == nil || standing.State != triage.Proposed {
			t.Error("undoing an approval withdrew the claim as well")
		}
	})
}
