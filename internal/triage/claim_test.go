package triage_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// secondIssue is another vulnerability, for a claim about a different issue
// at the same place.
func (f *fixture) secondIssue(t *testing.T) int64 {
	t.Helper()
	interned, err := finding.NewVulnerabilities(f.db.DB).Intern(t.Context(), []finding.Named{
		{Identifier: "CVE-2026-2", Severity: "low"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return interned["CVE-2026-2"]
}

// privately is somebody who may argue about undisclosed findings as well as
// disclosed ones.
func (f *fixture) privately(t *testing.T) access.Subject {
	t.Helper()
	ctx := t.Context()
	rights := access.NewStore(f.db.DB)
	insider, err := rights.Ensure(ctx, "insider", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := rights.GrantRole(ctx, insider.ID, f.product, access.PrivateTriage); err != nil {
		t.Fatal(err)
	}
	resolved, err := rights.Resolve(ctx, "insider")
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// places is a finding sitting at several consumers: the shape one judgment
// covers in one action.
func (f *fixture) places(names ...string) []triage.Place {
	out := make([]triage.Place, 0, len(names))
	for _, name := range names {
		at := f.at()
		at.PlaceIdentity = name
		out = append(out, at)
	}
	return out
}

// keyed is what a finding asks about: its places, with the versions it holds
// at each — the fixture's defaults, so they match what claims wrote.
func (f *fixture) keyed(names ...string) []finding.Deciding {
	out := make([]finding.Deciding, 0, len(names))
	for _, at := range f.places(names...) {
		out = append(out, finding.Deciding{
			ProductID: at.ProductID, VulnerabilityID: at.VulnerabilityID,
			PlaceIdentity:     at.PlaceIdentity,
			ComponentUpstream: at.ComponentUpstream, ConsumerUpstream: at.ConsumerUpstream,
		})
	}
	return out
}

// claimsMany records one judgment across several places, which writes one
// claim and one decision per place.
func (f *fixture) claimsMany(t *testing.T, places []triage.Place) []*triage.Decision {
	t.Helper()
	proposals := make([]triage.Proposal, 0, len(places))
	for _, at := range places {
		proposals = append(proposals, triage.Proposal{
			Place: at, Outcome: triage.NotApplicable,
			Justification: triage.CodeNotInExecutePath,
			Reasoning:     "The parser is never reached: we only call the encoder.",
			By:            f.proposer, NeedsApproval: true,
		})
	}
	recorded, err := f.store.ProposeMany(t.Context(), f.triager, proposals)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func TestOneActionIsOneClaimHoweverManyPlacesItCovers(t *testing.T) {
	// The queue lists claims, not rows. A finding at sixty-two places is one
	// argument to read, and it was sixty-two identical cards (TRI-45).
	each(t, func(t *testing.T, f *fixture) {
		recorded := f.claimsMany(t, f.places("under-a", "under-b", "under-c"))
		for _, one := range recorded[1:] {
			if one.ClaimID != recorded[0].ClaimID {
				t.Fatalf("one action wrote rows under claims %d and %d", recorded[0].ClaimID, one.ClaimID)
			}
		}

		waiting, total, err := f.store.Queue(t.Context(), f.reviewer, false, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(waiting) != 1 {
			t.Fatalf("%d waiting (total %d), want one claim", len(waiting), total)
		}
		if waiting[0].Decisions != 3 || waiting[0].Places != 3 || waiting[0].Issues != 1 {
			t.Errorf("the claim reads as %d decisions, %d places, %d issues; want 3, 3, 1",
				waiting[0].Decisions, waiting[0].Places, waiting[0].Issues)
		}
		if waiting[0].Claim.Kind != triage.FindingClaim {
			t.Errorf("a judgment about a finding reads as a %q claim", waiting[0].Claim.Kind)
		}
	})
}

func TestApprovingAClaimApprovesEveryRowInIt(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		recorded := f.claimsMany(t, f.places("under-a", "under-b"))

		done, err := f.store.ApproveClaim(ctx, f.reviewer, recorded[0].ClaimID, "", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if done.Approved != 2 || done.Returned != nil {
			t.Errorf("approving the claim agreed to %d rows and returned %v", done.Approved, done.Returned)
		}
		for _, at := range f.places("under-a", "under-b") {
			if standing, _ := f.store.Applying(ctx, at); standing == nil || standing.State != triage.Approved {
				t.Errorf("after approving the claim, nothing approved stands at %s", at.PlaceIdentity)
			}
		}
		if _, total, _ := f.store.Queue(ctx, f.reviewer, false, 50, 0); total != 0 {
			t.Errorf("an approved claim is still waiting: %d in the queue", total)
		}
	})
}

func TestTheProposerMayNotApproveTheirOwnClaim(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		recorded := f.claimsMany(t, f.places("under-a"))
		_, err := f.store.ApproveClaim(t.Context(), f.triager, recorded[0].ClaimID, "", nil, "")
		if !errors.Is(err, triage.ErrSamePerson) {
			t.Errorf("the proposer approving their own claim answered %v", err)
		}
	})
}

func TestRowsSetAsideReturnAsTheirOwnClaim(t *testing.T) {
	// An approver of a bulk claim who found the handful that do not fit
	// should not have to choose between refusing everything and agreeing to
	// everything: the rest is approved, and those go back with the reason
	// (TRI-46).
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		recorded := f.claimsMany(t, f.places("under-a", "under-b", "under-c"))
		aside := recorded[2]

		done, err := f.store.ApproveClaim(ctx, f.reviewer, recorded[0].ClaimID, "",
			[]int64{aside.ID}, "This one is reachable from the CLI.")
		if err != nil {
			t.Fatal(err)
		}
		if done.Approved != 2 {
			t.Errorf("approved %d rows, want the 2 not set aside", done.Approved)
		}
		if done.Returned == nil || done.Returned.Kind != triage.ReturnedClaim ||
			done.Returned.DerivedFrom == nil || *done.Returned.DerivedFrom != recorded[0].ClaimID {
			t.Fatalf("the rows set aside did not return as a claim derived from the original: %+v", done.Returned)
		}
		if done.Returned.ProposedBy != f.proposer {
			t.Errorf("the returned claim belongs to %d, not to the proposer %d", done.Returned.ProposedBy, f.proposer)
		}

		moved, _, err := f.store.Read(ctx, f.reviewer, aside.ID)
		if err != nil {
			t.Fatal(err)
		}
		if moved.ClaimID != done.Returned.ID {
			t.Errorf("the row set aside still belongs to claim %d", moved.ClaimID)
		}
		if moved.State != triage.Proposed || moved.SentBackAt == nil {
			t.Errorf("the row set aside reads as %s and sent back at %v", moved.State, moved.SentBackAt)
		}
		said, err := f.store.Discussion(ctx, f.reviewer, aside.ID)
		if err != nil || len(said) != 1 || said[0].Body != "This one is reachable from the CLI." {
			t.Errorf("the reason did not travel as a comment: %v %v", said, err)
		}
		// Sent back, so not waiting on an approver; and the approved rest is
		// not waiting either.
		if _, total, _ := f.store.Queue(ctx, f.reviewer, false, 50, 0); total != 0 {
			t.Errorf("%d claims still waiting after approving with rows set aside", total)
		}
	})
}

func TestSettingRowsAsideNeedsAReason(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		recorded := f.claimsMany(t, f.places("under-a", "under-b"))
		_, err := f.store.ApproveClaim(t.Context(), f.reviewer, recorded[0].ClaimID, "",
			[]int64{recorded[1].ID}, "")
		if err == nil || !strings.Contains(err.Error(), "set aside") {
			t.Errorf("rows were set aside with nothing said about why: %v", err)
		}
		_, err = f.store.ApproveClaim(t.Context(), f.reviewer, recorded[0].ClaimID, "",
			[]int64{999999}, "Not one of ours.")
		if err == nil {
			t.Error("a row that is not part of the claim was accepted as set aside")
		}
	})
}

func TestSendingAClaimBackTakesEveryRowOutOfTheQueue(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		recorded := f.claimsMany(t, f.places("under-a", "under-b"))
		back, err := f.store.SendBackClaim(ctx, f.reviewer, recorded[0].ClaimID,
			"Say which config file.")
		if err != nil {
			t.Fatal(err)
		}
		if back.Sent != 2 || len(back.Authors) != 1 || back.Authors[0] != f.proposer {
			t.Errorf("sent %d rows back to %v; want 2 to %d", back.Sent, back.Authors, f.proposer)
		}
		if back.Decision.ID != recorded[0].ID {
			t.Errorf("the representative row is %d; want the earliest, %d", back.Decision.ID, recorded[0].ID)
		}
		if _, total, _ := f.store.Queue(ctx, f.reviewer, false, 50, 0); total != 0 {
			t.Errorf("a claim sent back is still waiting: %d", total)
		}
	})
}

func TestAnExtensionCarriesOnlyAnApprovedClaimToTheSamePlaces(t *testing.T) {
	// The nightly case: a new issue in a component under a consumer that
	// already carries an agreed claim with the same justification (TRI-47).
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		source := f.claimsMany(t, f.places("under-a", "under-b"))

		another := f.at()
		another.VulnerabilityID = f.secondIssue(t)
		another.PlaceIdentity = "under-a"
		extension := triage.Proposal{
			Place: another, Outcome: triage.NotApplicable,
			Justification: triage.CodeNotInExecutePath,
			Reasoning:     "Same code path as the last one; still never reached.",
			By:            f.proposer, NeedsApproval: true,
		}

		// Not while the source is only proposed.
		if _, err := f.store.Extend(ctx, f.triager, source[0].ClaimID,
			[]triage.Proposal{extension}); !errors.Is(err, triage.ErrNotExtendable) {
			t.Errorf("a claim nobody agreed to was carried: %v", err)
		}
		if _, err := f.store.ApproveClaim(ctx, f.reviewer, source[0].ClaimID, "", nil, ""); err != nil {
			t.Fatal(err)
		}

		// Not to a different place.
		elsewhere := extension
		elsewhere.Place.PlaceIdentity = "under-z"
		if _, err := f.store.Extend(ctx, f.triager, source[0].ClaimID,
			[]triage.Proposal{elsewhere}); !errors.Is(err, triage.ErrNotExtendable) {
			t.Errorf("a claim was carried to a place it does not sit at: %v", err)
		}
		// Not with a different justification.
		differently := extension
		differently.Justification = triage.ComponentNotPresent
		if _, err := f.store.Extend(ctx, f.triager, source[0].ClaimID,
			[]triage.Proposal{differently}); !errors.Is(err, triage.ErrNotExtendable) {
			t.Errorf("a claim was carried with a different justification: %v", err)
		}
		// Not to the issue it already covers.
		same := extension
		same.Place.VulnerabilityID = f.issue
		if _, err := f.store.Extend(ctx, f.triager, source[0].ClaimID,
			[]triage.Proposal{same}); !errors.Is(err, triage.ErrNotExtendable) {
			t.Errorf("a claim was carried to the issue it already covers: %v", err)
		}

		recorded, err := f.store.Extend(ctx, f.triager, source[0].ClaimID, []triage.Proposal{extension})
		if err != nil {
			t.Fatal(err)
		}
		waiting, _, err := f.store.Queue(ctx, f.reviewer, false, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(waiting) != 1 || waiting[0].Claim.ID != recorded[0].ClaimID {
			t.Fatalf("the extension is not what is waiting: %+v", waiting)
		}
		if waiting[0].Claim.Kind != triage.ExtensionClaim ||
			waiting[0].Claim.DerivedFrom == nil || *waiting[0].Claim.DerivedFrom != source[0].ClaimID {
			t.Errorf("the extension does not say what it carries: %+v", waiting[0].Claim)
		}
		// And it still needs a second person: nothing stands at the new issue
		// until somebody agrees.
		if standing, _ := f.store.Applying(ctx, another); standing != nil {
			t.Error("an extension suppressed the new issue before anybody agreed")
		}
	})
}

func TestAFindingReportsWhatStandsWhatStoodAndWhatMightCarry(t *testing.T) {
	// What somebody returning to a finding asks: what stands here, what was
	// argued before, and whether an argument already agreed to reaches this
	// (UIX-46, TRI-47).
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		places := []string{"under-a", "under-b"}

		// Nothing yet.
		standing, err := f.store.StandingAt(ctx, f.reviewer, f.product, f.issue, f.keyed(places...))
		if err != nil || len(standing) != 0 {
			t.Fatalf("an undecided finding reports %d standing claims (%v)", len(standing), err)
		}

		first := f.claimsMany(t, f.places(places...))
		standing, err = f.store.StandingAt(ctx, f.reviewer, f.product, f.issue, f.keyed(places...))
		if err != nil || len(standing) != 1 {
			t.Fatalf("a proposed claim is not reported as standing: %d (%v)", len(standing), err)
		}
		if standing[0].Places != 2 || standing[0].Decision.State != triage.Proposed || standing[0].ApprovedAt != nil {
			t.Errorf("a proposed claim reads as %+v", standing[0])
		}
		if _, err := f.store.ApproveClaim(ctx, f.reviewer, first[0].ClaimID, "", nil, ""); err != nil {
			t.Fatal(err)
		}
		standing, _ = f.store.StandingAt(ctx, f.reviewer, f.product, f.issue, f.keyed(places...))
		if len(standing) != 1 || standing[0].ApprovedBy != f.approver || standing[0].ApprovedAt == nil {
			t.Errorf("an approved claim does not say who agreed: %+v", standing)
		}

		// Withdrawn, it is history with a date — and offered back.
		if err := f.store.Withdraw(ctx, f.triager, first[0].ID); err != nil {
			t.Fatal(err)
		}
		earlier, err := f.store.EarlierAt(ctx, f.reviewer, f.product, f.issue, places)
		if err != nil || len(earlier) != 1 {
			t.Fatalf("a withdrawn decision is not offered back: %d (%v)", len(earlier), err)
		}
		if earlier[0].Decision.State != triage.Withdrawn || earlier[0].Decision.EndedAt == nil {
			t.Errorf("a withdrawn decision does not say when: %+v", earlier[0].Decision)
		}
		if earlier[0].Reasoning == "" || earlier[0].ApprovedBy != f.approver {
			t.Errorf("what was argued, and who agreed, did not come back: %+v", earlier[0])
		}

		// An approved claim about another issue at the same place may carry.
		other := f.at()
		other.VulnerabilityID = f.secondIssue(t)
		other.PlaceIdentity = "under-b"
		agreed := f.claimsMany(t, []triage.Place{other})
		if _, err := f.store.ApproveClaim(ctx, f.reviewer, agreed[0].ClaimID, "", nil, ""); err != nil {
			t.Fatal(err)
		}
		similar, err := f.store.SimilarAt(ctx, f.reviewer, f.product, f.issue, places)
		if err != nil || len(similar) != 1 || similar[0].Claim.ID != agreed[0].ClaimID {
			t.Fatalf("the approved claim about the other issue is not offered: %+v (%v)", similar, err)
		}
		if similar[0].Issues != 1 || similar[0].ApprovedBy != f.approver || similar[0].Reasoning == "" {
			t.Errorf("what may carry is missing its size, its approver or its words: %+v", similar[0])
		}
	})
}

func TestAClaimIsShownOnlyToSomebodyWhoMayActOnAllOfIt(t *testing.T) {
	// Acting on a claim is acting on the argument, which does not come in
	// halves. A claim spanning a disclosed and an undisclosed finding is not
	// shown to somebody who may agree to only the disclosed half.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		open := f.at()
		open.PlaceIdentity = "under-a"
		hidden := f.at()
		hidden.PlaceIdentity = "under-b"
		hidden.Visibility = access.Private

		insider := f.privately(t)
		proposals := []triage.Proposal{}
		for _, at := range []triage.Place{open, hidden} {
			proposals = append(proposals, triage.Proposal{
				Place: at, Outcome: triage.WontFix,
				Reasoning: "Not worth the change.", By: insider.ID, NeedsApproval: true,
			})
		}
		if _, err := f.store.ProposeMany(ctx, insider, proposals); err != nil {
			t.Fatal(err)
		}

		// The public reviewer may agree to one row and not the other, so the
		// claim is not theirs to act on.
		if _, total, err := f.store.Queue(ctx, f.reviewer, false, 50, 0); err != nil || total != 0 {
			t.Errorf("a claim half of which is undisclosed was shown to a public reviewer: %d (%v)", total, err)
		}
		// The person who proposed it is not shown it either, and for a
		// different reason: approving your own is refused, so a queue holding
		// it would be a list of work they cannot do.
		if _, total, err := f.store.Queue(ctx, insider, false, 50, 0); err != nil || total != 0 {
			t.Errorf("somebody was shown their own claim in the queue: %d (%v)", total, err)
		}
		// They find it by asking for their own, which is the other question:
		// what did I propose that nobody has agreed to. Without this the two
		// assertions above would pass on a queue that shows nothing to
		// anybody.
		if _, total, err := f.store.Queue(ctx, insider, true, 50, 0); err != nil || total != 1 {
			t.Errorf("a claim's proposer could not find it among their own: %d (%v)", total, err)
		}
	})
}

// counting is a query hook that counts statements, so a test can pin that an
// action is a bounded number of them however many rows it touches.
type counting struct{ n int }

func (c *counting) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	c.n++
	return ctx
}
func (c *counting) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestApprovingAClaimIsABoundedNumberOfStatementsHoweverLargeItIs(t *testing.T) {
	// A claim over a kernel is two thousand rows. Approved one row at a time
	// that is a count and a conditional update per row, which was measured at
	// 15.6 s for 1,760 rows on the demo; approved as a set it is a handful
	// of statements whatever the size. The invariant pinned here is the
	// statement count, which is what a per-row loop cannot satisfy.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		names := make([]string, 0, 500)
		for i := 0; i < 500; i++ {
			names = append(names, fmt.Sprintf("under-%03d", i))
		}
		recorded := f.claimsMany(t, f.places(names...))

		hook := &counting{}
		f.db.AddQueryHook(hook)
		started := time.Now()
		done, err := f.store.ApproveClaim(ctx, f.reviewer, recorded[0].ClaimID, "batch",
			[]int64{recorded[0].ID, recorded[1].ID}, "These two are reachable.")
		elapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("approving 500 rows with two set aside: %d statements in %s", hook.n, elapsed)
		if done.Approved != 498 || done.Returned == nil {
			t.Fatalf("approved %d and returned %v", done.Approved, done.Returned)
		}
		// Generous, and independent of the row count: a per-row loop is at
		// least a thousand here.
		if hook.n > 24 {
			t.Errorf("approving a 500-row claim took %d statements; the work has to be set-based", hook.n)
		}
		// Every row that was meant to be approved is, with its approval
		// naming the revision it stands on and what it covered.
		var approvals []triage.Approval
		if err := f.db.DB.NewSelect().Model(&approvals).Where("batch = ?", "batch").Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(approvals) != 498 {
			t.Errorf("%d approvals recorded, want 498", len(approvals))
		}
		for _, a := range approvals {
			if a.Covered == nil || a.RevisionID == 0 {
				t.Fatalf("an approval was recorded without what it covered or which words: %+v", a)
			}
		}
	})
}

func TestAStandingClaimReportsHowItsRowsStandNotOneRowsState(t *testing.T) {
	// One row approved beside forty-three sent back read as "approved",
	// because the representative row's state stood in for the claim's.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		places := []string{"under-a", "under-b", "under-c"}
		recorded := f.claimsMany(t, f.places(places...))

		// Send one row back on its own, so one claim mixes waiting and
		// returned rows.
		if _, _, err := f.store.SendBack(ctx, f.reviewer, recorded[2].ID, "Which config file?"); err != nil {
			t.Fatal(err)
		}
		standing, err := f.store.StandingAt(ctx, f.reviewer, f.product, f.issue, f.keyed(places...))
		if err != nil || len(standing) != 1 {
			t.Fatalf("standing reads as %+v (%v)", standing, err)
		}
		one := standing[0]
		if one.State != triage.Proposed || one.Rows.Proposed != 2 || one.Rows.SentBack != 1 || one.Rows.Approved != 0 {
			t.Errorf("a claim with a row sent back reads as %s with rows %+v", one.State, one.Rows)
		}
		if one.SentBackAt == nil || one.SentBackBecause != "Which config file?" {
			t.Errorf("what the approver asked for did not come back: %v %q", one.SentBackAt, one.SentBackBecause)
		}

		// Approve the two waiting rows: the claim is still not approved as a
		// whole, because one row is with the author.
		if _, err := f.store.ApproveClaim(ctx, f.reviewer, recorded[0].ClaimID, "", nil, ""); err != nil {
			t.Fatal(err)
		}
		standing, _ = f.store.StandingAt(ctx, f.reviewer, f.product, f.issue, f.keyed(places...))
		if len(standing) != 1 || standing[0].State != triage.Proposed || standing[0].Rows.Approved != 2 {
			t.Errorf("after approving the waiting rows the claim reads as %+v", standing)
		}
	})
}
