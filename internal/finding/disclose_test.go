package finding_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestAnEmbargoGetsAnEndAndIsSurfacedBeforeItArrives(t *testing.T) {
	// ACC-46 and ACC-49. An embargo with no end is the indefinite secrecy the
	// disclosure frameworks warn about, arrived at by nobody deciding
	// anything. And the date arriving is the last moment to act on it, not the
	// first useful warning — a list that only ever showed what was already
	// past would be a list of decisions somebody has already failed to make.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		who := f.planner(t, access.PrivateTriage)

		row, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Severity: "high",
			Summary: "The management socket answers before anyone authenticated.",
		})
		if err != nil {
			t.Fatal(err)
		}
		if row.DiscloseAt == nil {
			t.Fatal("an undisclosed finding has no end to its embargo")
		}
		if got := row.DiscloseAt.Sub(row.OpenedAt); (got - 90*24*time.Hour).Abs() > time.Minute {
			t.Errorf("the embargo runs %s, want the ninety days a deployment starts with", got)
		}

		// A month out it is not on a thirty-day list yet, because ninety days
		// is further away than that.
		soon, err := f.store.Disclosing(t.Context(), who, finding.Scope{},
			30*24*time.Hour, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(soon) != 0 {
			t.Errorf("something ninety days out is on a thirty-day list: %+v", soon)
		}

		// Wider than the embargo, and it is there — before the date, with what
		// somebody needs to act on it.
		ahead, err := f.store.Disclosing(t.Context(), who, finding.Scope{},
			120*24*time.Hour, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(ahead) != 1 {
			t.Fatalf("%d findings are approaching disclosure, want 1", len(ahead))
		}
		if ahead[0].Summary == "" || ahead[0].Product == "" || ahead[0].Component == "" {
			t.Errorf("the row does not say enough to act on: %+v", ahead[0])
		}
		if ahead[0].Passed(time.Now().UTC()) {
			t.Error("a date ninety days away reads as passed")
		}

		// A disclosed finding carries no date: it is already public, and a
		// date on it would be a deadline for something that has happened.
		disclosed, _, err := f.store.Enter(t.Context(), f.planner(t, access.PrivateTriage),
			finding.Entering{
				TargetID: f.target, Severity: "high", Disclosed: true,
				Summary: "Already announced.",
			})
		if err != nil {
			t.Fatal(err)
		}
		if disclosed.DiscloseAt != nil {
			t.Errorf("a public finding carries an embargo ending %s", disclosed.DiscloseAt)
		}
	})
}

func TestWhatIsApproachingDisclosureIsItselfUndisclosed(t *testing.T) {
	// Every row on this list is a finding nobody has announced, so the list is
	// a disclosure in its own right. Somebody who may not read undisclosed
	// work in a product sees none of that product's — and a count would say as
	// much as a row.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, _, err := f.store.Enter(t.Context(), f.planner(t, access.PrivateTriage),
			finding.Entering{
				TargetID: f.target, Severity: "critical",
				Summary: "Something nobody outside knows about.",
			}); err != nil {
			t.Fatal(err)
		}

		for _, role := range []access.Role{access.PublicRead, access.PublicTriage} {
			rows, err := f.store.Disclosing(t.Context(), f.planner(t, role),
				finding.Scope{}, 365*24*time.Hour, 50)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 0 {
				t.Errorf("somebody holding %s was shown %d undisclosed findings", role, len(rows))
			}
		}

		rows, err := f.store.Disclosing(t.Context(), f.planner(t, access.PrivateRead),
			finding.Scope{}, 365*24*time.Hour, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Errorf("somebody who may read undisclosed work sees %d, want 1", len(rows))
		}
	})
}
