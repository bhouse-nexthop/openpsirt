package finding_test

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// fixture is one migrated database with a variant, a stored graph, and a way
// to mint scan runs in order.
type fixture struct {
	db     *database.DB
	store  *finding.Store
	graph  *graph.Store
	target int64
	scans  *ingest.Store
	built  time.Time
	seq    int
}

func at(name, version string) graph.Described {
	return graph.Described{
		Purl: "pkg:deb/debian/" + name + "@" + version, Name: name, Version: version,
	}
}

var (
	root     = at("sonic", "1.0")
	swss     = at("libswsscommon", "1.0.0")
	teamd    = at("teamd", "1.31")
	libnl    = at("libnl-3-200", "3.7.0")
	libnlNew = at("libnl-3-200", "3.9.0")
)

// shipped stores a graph: libnl sits under two consumers, which is the case
// the whole place definition exists for.
func (f *fixture) shipped(t *testing.T, snap graph.Snapshot) {
	t.Helper()
	f.seq++
	f.built = f.built.Add(time.Hour)
	scan, outcome, err := f.scans.Record(t.Context(), ingest.Arriving{
		TargetID: f.target, ContentHash: fmt.Sprintf("hash-%d", f.seq), BuiltAt: f.built,
		ParserVersion: "test",
	})
	if err != nil || outcome != ingest.Accept {
		t.Fatalf("record scan: %v %v", outcome, err)
	}
	if _, err := f.graph.Apply(t.Context(), f.target, scan.ID, snap); err != nil {
		t.Fatalf("apply graph: %v", err)
	}
}

// twoConsumers is the graph most of these tests use.
func twoConsumers() graph.Snapshot {
	return graph.Snapshot{
		Root:       root,
		Components: []graph.Described{swss, teamd, libnl},
		Dependencies: []graph.Dependency{
			{Parent: root, Child: swss},
			{Parent: root, Child: teamd},
			{Parent: swss, Child: libnl},
			{Parent: teamd, Child: libnl},
		},
	}
}

// run starts a scan run and returns its identifier.
func (f *fixture) run(t *testing.T) int64 {
	t.Helper()
	r, err := f.store.Begin(t.Context(), finding.Run{
		TargetID: f.target, Scanner: "grype", ScannerVersion: "0.100.0",
		DatabaseVersion: "2026-08-28", RanHere: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

// found is one reported issue against a component.
func found(id string, component graph.Described, aliases ...string) finding.Reported {
	return finding.Reported{
		Issue:     finding.Named{Identifier: id, Aliases: aliases, Severity: "high"},
		Component: component,
		FixState:  finding.FixedUpstream, FixedIn: "3.9.0",
	}
}

func (f *fixture) open(t *testing.T) []finding.Finding {
	t.Helper()
	var rows []finding.Finding
	err := f.db.DB.NewSelect().Model(&rows).
		Where("target_id = ?", f.target).Where("closed_run_id IS NULL").Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return rows
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

		cat := catalog.NewStore(db.DB)
		product, err := cat.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		stream, err := cat.DeclareStream(ctx, product.ID, "master", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		variant, err := cat.DeclareVariant(ctx, product.ID, "broadcom", true)
		if err != nil {
			t.Fatal(err)
		}
		target, err := cat.TargetFor(ctx, stream.ID, variant.ID)
		if err != nil {
			t.Fatal(err)
		}

		fn(t, &fixture{
			db: db, store: finding.NewStore(db.DB), graph: graph.NewStore(db.DB),
			target: target.ID, scans: ingest.NewStore(db.DB),
			built: time.Now().UTC().Add(-72 * time.Hour),
		})
	})
}

func TestOneReportedIssueBecomesOneFindingPerConsumer(t *testing.T) {
	// The whole point of the fan-out. A scanner says "libnl-3-200 3.7.0 is
	// affected" and stops, because it never saw the graph. Two things pull it
	// in, so there are two decisions to make about it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Opened != 2 {
			t.Fatalf("opened %d findings, want one per consumer", applied.Opened)
		}

		consumers := map[int64]bool{}
		for _, row := range f.open(t) {
			if row.ConsumerID == nil {
				t.Error("a finding under a consumer recorded none")
				continue
			}
			consumers[*row.ConsumerID] = true
		}
		if len(consumers) != 2 {
			t.Errorf("findings sit under %d distinct consumers, want 2", len(consumers))
		}
	})
}

func TestTheSamePlaceInTwoVariantsKeysTheSame(t *testing.T) {
	// A decision is carried forward by the place, so the key must not contain
	// anything that differs between variants or moves on a rebuild.
	first := finding.PlaceIdentity("libnl-3-200", "libswsscommon")
	again := finding.PlaceIdentity("libnl-3-200", "libswsscommon")
	if first != again {
		t.Error("the same place keyed differently")
	}
	if finding.PlaceIdentity("libnl-3-200", "teamd") == first {
		t.Error("two consumers of one component share a key")
	}
	// Under the product itself, the component stands alone: the product's name
	// differs per variant, so including it would stop the same place being
	// recognized across them.
	if finding.PlaceIdentity("libnl-3-200", "") == first {
		t.Error("a component under the product keys the same as one under a consumer")
	}
}

func TestRescanningWithNothingChangedWritesNothing(t *testing.T) {
	// Re-scanning runs nightly against a database that has barely moved. If an
	// unchanged run wrote rows, storage would track the calendar rather than
	// what is actually happening.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		reported := []finding.Reported{found("CVE-2026-1", libnl)}

		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), reported); err != nil {
			t.Fatal(err)
		}
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), reported)
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Unchanged() {
			t.Errorf("an unchanged re-scan wrote %+v", applied)
		}
	})
}

func TestAnUpgradeClosesWithTheReason(t *testing.T) {
	// "Fixed by upgrading" and "we cannot account for this" are different
	// things to whoever reads the report later.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		// The next build ships a newer libnl, and the scanner no longer
		// reports it.
		upgraded := twoConsumers()
		upgraded.Components = []graph.Described{swss, teamd, libnlNew}
		upgraded.Dependencies = []graph.Dependency{
			{Parent: root, Child: swss}, {Parent: root, Child: teamd},
			{Parent: swss, Child: libnlNew}, {Parent: teamd, Child: libnlNew},
		}
		f.shipped(t, upgraded)

		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		if applied.Closed != 2 {
			t.Fatalf("closed %d findings, want 2", applied.Closed)
		}
		if applied.Unexplained != 0 {
			t.Errorf("%d closures went unexplained", applied.Unexplained)
		}

		var closed []finding.Finding
		if err := f.db.DB.NewSelect().Model(&closed).
			Where("closed_run_id IS NOT NULL").Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		for _, row := range closed {
			if row.ClosedBecause != finding.Upgraded {
				t.Errorf("closed because %q, want %q", row.ClosedBecause, finding.Upgraded)
			}
		}
	})
}

func TestAComponentGoneAltogetherClosesAsRemoved(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		f.shipped(t, graph.Snapshot{
			Root: root, Components: []graph.Described{swss, teamd},
			Dependencies: []graph.Dependency{{Parent: root, Child: swss}, {Parent: root, Child: teamd}},
		})
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), nil); err != nil {
			t.Fatal(err)
		}

		var closed []finding.Finding
		if err := f.db.DB.NewSelect().Model(&closed).
			Where("closed_run_id IS NOT NULL").Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(closed) != 2 {
			t.Fatalf("closed %d findings, want 2", len(closed))
		}
		for _, row := range closed {
			if row.ClosedBecause != finding.Removed {
				t.Errorf("closed because %q, want %q", row.ClosedBecause, finding.Removed)
			}
		}
	})
}

func TestADisappearanceNothingExplainsIsFlagged(t *testing.T) {
	// The component is present, unchanged, and the scanner stopped reporting
	// it. There is no volume at which "we cannot account for this" stops
	// mattering, so it is never quietly folded into the others.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		if applied.Unexplained != 2 {
			t.Errorf("%d closures were flagged as unexplained, want 2", applied.Unexplained)
		}
	})
}

func TestOneIssueUnderTwoNamesIsOneIssue(t *testing.T) {
	// Which identifier a scanner calls primary is a preference of whichever
	// database it consulted. A decision keyed on that choice would lapse the
	// day the scanner changed its mind.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())

		// First run: reported under an advisory identifier that knows the
		// national one as an alias.
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("GHSA-aaaa-bbbb-cccc", libnl, "CVE-2026-1")}); err != nil {
			t.Fatal(err)
		}
		opened := f.open(t)
		if len(opened) != 2 {
			t.Fatalf("opened %d findings", len(opened))
		}

		// Second run: the same issue, reported the other way round.
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)})
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Unchanged() {
			t.Errorf("the same issue under another name wrote %+v", applied)
		}

		var issues int
		if issues, err = f.db.DB.NewSelect().Model((*finding.Vulnerability)(nil)).Count(t.Context()); err != nil {
			t.Fatal(err)
		}
		if issues != 1 {
			t.Errorf("%d vulnerabilities recorded, want 1", issues)
		}
	})
}

func TestAnAliasSuppliedLaterFindsTheIssueAlreadyHeld(t *testing.T) {
	// The order that matters. One scanner reports the national identifier and
	// nothing else; another later reports its own identifier and knows the
	// first as an alias. Only looking up the name a report happened to lead
	// with would make those two issues, splitting the findings and every
	// decision between them.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())

		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("GHSA-aaaa-bbbb-cccc", libnl, "CVE-2026-1")})
		if err != nil {
			t.Fatalf("an issue reported under a second name: %v", err)
		}
		if !applied.Unchanged() {
			t.Errorf("the same issue under a second name wrote %+v", applied)
		}

		issues, err := f.db.DB.NewSelect().Model((*finding.Vulnerability)(nil)).Count(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if issues != 1 {
			t.Errorf("%d vulnerabilities recorded, want 1", issues)
		}
	})
}

func TestAnIssueIsFiledUnderItsMostRecognisedName(t *testing.T) {
	// What somebody sees should be the name they will find in an advisory,
	// not whichever database the scanner happened to consult first.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("GHSA-aaaa-bbbb-cccc", libnl, "CVE-2026-1")}); err != nil {
			t.Fatal(err)
		}
		var held finding.Vulnerability
		if err := f.db.DB.NewSelect().Model(&held).Limit(1).Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		if held.Identifier != "CVE-2026-1" {
			t.Errorf("filed under %q, want the national identifier", held.Identifier)
		}
	})
}

func TestAnIssueAgainstSomethingWeDoNotHaveIsReported(t *testing.T) {
	// A report that does not match the inventory it was produced from is
	// worth seeing, not quietly discarding.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-9", at("openssl", "3.0.11"))})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Unplaced != 1 || applied.Opened != 0 {
			t.Errorf("applied %+v, want one unplaced and nothing opened", applied)
		}
	})
}

func TestAComponentUnderTheProductSitsUnderNothing(t *testing.T) {
	// The product's name differs per variant, so a place under it keys on the
	// component alone — otherwise the same place in two variants would be two
	// places and a decision would not carry.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-2", swss)}); err != nil {
			t.Fatal(err)
		}
		rows := f.open(t)
		if len(rows) != 1 {
			t.Fatalf("opened %d findings, want 1", len(rows))
		}
		if rows[0].ConsumerID != nil {
			t.Error("a component under the product recorded a consumer")
		}
		if rows[0].PlaceIdentity != finding.PlaceIdentity("libswsscommon", "") {
			t.Error("a component under the product keyed on something else")
		}
	})
}
