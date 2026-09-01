package finding_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// fixture is one migrated database with a variant, a stored graph, and a way
// to mint scan runs in order.
type fixture struct {
	db        *database.DB
	store     *finding.Store
	graph     *graph.Store
	target    int64
	productID int64
	lastScan  int64
	scans     *ingest.Store
	built     time.Time
	seq       int
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
	f.lastScan = scan.ID
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
			target: target.ID, productID: product.ID, scans: ingest.NewStore(db.DB),
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

func TestAnIssueIsFiledUnderItsMostRecognizedName(t *testing.T) {
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

// aClaim is what a build argues about one of its components.
func aClaim(vulnerability string, status sbom.Status, subject graph.Described, origin sbom.Origin) sbom.Suppression {
	return sbom.Suppression{
		Vulnerability: vulnerability, Status: status,
		Justification: "vulnerable_code_not_in_execute_path",
		Statement:     "resolved by a patch the build carries",
		Targets:       []sbom.Target{{Purl: subject.Purl, Name: subject.Name}},
		Origin:        origin,
	}
}

func TestAFindingTheBuildHasAnsweredIsMarkedNotDropped(t *testing.T) {
	// The whole reason the claims are applied here rather than upstream. A
	// finding a build has argued about stays visible and says what was
	// argued; one that simply never arrived is indistinguishable from a
	// scanner that failed, and lands in the bucket nothing may explain away.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		scanID := f.lastScan
		if _, err := f.store.RecordClaims(t.Context(), f.target, scanID,
			[]sbom.Suppression{aClaim("CVE-2026-1", sbom.AlreadyFixed, libnl, sbom.FromPedigree)}); err != nil {
			t.Fatal(err)
		}

		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Opened != 2 {
			t.Fatalf("opened %d findings, want one per consumer even though the build answered them", applied.Opened)
		}
		if applied.Suppressed != 2 {
			t.Errorf("%d findings carry what the build argued, want 2", applied.Suppressed)
		}
		for _, row := range f.open(t) {
			if row.SuppressedBy == nil {
				t.Error("a finding the build answered does not say so")
			}
		}
	})
}

func TestAClaimAboutSomethingElseLeavesAFindingAlone(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan,
			[]sbom.Suppression{
				// Right component, different issue.
				aClaim("CVE-2026-9", sbom.NotAffected, libnl, sbom.FromStatement),
				// Right issue, different component.
				aClaim("CVE-2026-1", sbom.NotAffected, swss, sbom.FromStatement),
			}); err != nil {
			t.Fatal(err)
		}
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Suppressed != 0 {
			t.Errorf("%d findings were marked by a claim about something else", applied.Suppressed)
		}
	})
}

func TestSayingItIsAffectedSuppressesNothing(t *testing.T) {
	// A build saying it is affected, or that it has not decided, is telling us
	// it looked. That is information, not an answer.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan,
			[]sbom.Suppression{aClaim("CVE-2026-1", sbom.Affected, libnl, sbom.FromStatement)}); err != nil {
			t.Fatal(err)
		}
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Suppressed != 0 {
			t.Errorf("a build saying it is affected suppressed %d findings", applied.Suppressed)
		}
	})
}

func TestArguingTheSameThingAgainWritesNothing(t *testing.T) {
	// A build argues the same things night after night.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		claims := []sbom.Suppression{aClaim("CVE-2026-1", sbom.AlreadyFixed, libnl, sbom.FromPedigree)}

		if _, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan, claims); err != nil {
			t.Fatal(err)
		}
		f.shipped(t, twoConsumers())
		applied, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan, claims)
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Unchanged() {
			t.Errorf("re-arguing the same claims wrote %+v", applied)
		}

		// Withdrawing one closes it rather than deleting it: what a release
		// argued is a question asked years later.
		applied, err = f.store.RecordClaims(t.Context(), f.target, f.lastScan, nil)
		if err != nil {
			t.Fatal(err)
		}
		if applied.Closed != 1 {
			t.Errorf("withdrawing a claim closed %d", applied.Closed)
		}
		open, err := f.store.OpenClaims(t.Context(), f.target)
		if err != nil || len(open) != 0 {
			t.Errorf("%d claims still open (%v)", len(open), err)
		}
	})
}

func TestAClaimAttachedToItsComponentWinsOverOneThatNamedIt(t *testing.T) {
	// A claim that arrived on the component knows exactly what it is about. A
	// claim in a document may name a whole source tree and match by name, so
	// where both cover a finding the precise one is the one recorded.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		// Recorded in two steps so the vaguer claim is the older one. If
		// preferring the precise one were removed, the older would win, and a
		// test that relied on insertion order within a single call would pass
		// or fail depending on what a map felt like doing.
		vague := aClaim("CVE-2026-1", sbom.NotAffected, libnl, sbom.FromStatement)
		precise := aClaim("CVE-2026-1", sbom.AlreadyFixed, libnl, sbom.FromPedigree)
		if _, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan,
			[]sbom.Suppression{vague}); err != nil {
			t.Fatal(err)
		}
		f.shipped(t, twoConsumers())
		if _, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan,
			[]sbom.Suppression{vague, precise}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		open, err := f.store.OpenClaims(t.Context(), f.target)
		if err != nil {
			t.Fatal(err)
		}
		attached := map[int64]bool{}
		for _, claim := range open {
			if claim.Origin == string(sbom.FromPedigree) {
				attached[claim.ID] = true
			}
		}
		for _, row := range f.open(t) {
			if row.SuppressedBy == nil || !attached[*row.SuppressedBy] {
				t.Error("a finding recorded the vaguer of two claims that covered it")
			}
		}
	})
}

func TestAFindingThatMovesIsUpdated(t *testing.T) {
	// Somebody waiting on a fix is waiting for exactly this. A finding opened
	// when no fix existed would otherwise report that indefinitely, however
	// many times it was re-scanned.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		noFix := finding.Reported{
			Issue:     finding.Named{Identifier: "CVE-2026-1", Severity: "high"},
			Component: libnl, FixState: finding.NoFix,
		}
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{noFix}); err != nil {
			t.Fatal(err)
		}
		opened := f.open(t)
		if len(opened) != 2 {
			t.Fatalf("opened %d findings", len(opened))
		}
		was := opened[0].LastChangedAt

		// The next run knows a fix exists.
		fixed := noFix
		fixed.FixState = finding.FixedUpstream
		fixed.FixedIn = "3.9.0"
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{fixed})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Updated != 2 {
			t.Errorf("%d findings moved, want 2", applied.Updated)
		}
		if applied.Opened != 0 || applied.Closed != 0 {
			t.Errorf("a fix appearing opened %d and closed %d findings", applied.Opened, applied.Closed)
		}
		if applied.Unchanged() {
			t.Error("a fix appearing reported as no change at all")
		}

		for _, row := range f.open(t) {
			if row.FixState != finding.FixedUpstream || row.FixedIn != "3.9.0" {
				t.Errorf("still reports %q / %q", row.FixState, row.FixedIn)
			}
			if !row.LastChangedAt.After(was) {
				t.Error("nothing recorded that it moved")
			}
		}

		// And a run that finds the same thing again still writes nothing.
		applied, err = f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{fixed})
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Unchanged() {
			t.Errorf("an unchanged re-scan wrote %+v", applied)
		}

	})
}

func TestUpstreamDecliningToFixIsAMovement(t *testing.T) {
	// The only thing that changes is the state — there is no version to point
	// at either before or after. It is a permanent condition that changes the
	// outcome somebody should reach, so it must not be the one kind of
	// movement that slips past because nothing else moved with it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		noFix := finding.Reported{
			Issue:     finding.Named{Identifier: "CVE-2026-1", Severity: "high"},
			Component: libnl, FixState: finding.NoFix,
		}
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{noFix}); err != nil {
			t.Fatal(err)
		}

		declined := noFix
		declined.FixState = finding.WontFix
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{declined})
		if err != nil {
			t.Fatal(err)
		}
		if applied.Updated != 2 {
			t.Errorf("upstream declining to fix moved %d findings, want 2", applied.Updated)
		}
		for _, row := range f.open(t) {
			if row.FixState != finding.WontFix {
				t.Errorf("still reports %q", row.FixState)
			}
		}
	})
}

func TestEveryStatusTheFormatDefinesCanBeStored(t *testing.T) {
	// The vocabulary belongs to the exchange format rather than to us, and its
	// longest word is longer than a short identifier column allows. A claim
	// that cannot be stored fails the whole scan that carried it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		var claims []sbom.Suppression
		for _, status := range []sbom.Status{
			sbom.NotAffected, sbom.Affected, sbom.AlreadyFixed, sbom.UnderInvestigation,
		} {
			claim := aClaim("CVE-2026-1", status, libnl, sbom.FromStatement)
			claim.Justification = "vulnerable_code_cannot_be_controlled_by_adversary"
			claims = append(claims, claim)
		}
		applied, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan, claims)
		if err != nil {
			t.Fatalf("recording every status the format defines: %v", err)
		}
		if applied.Opened != 4 {
			t.Errorf("stored %d claims, want one per status", applied.Opened)
		}
	})
}

func TestAClaimThatReachedNothingIsCounted(t *testing.T) {
	// The ordinary case, not the exceptional one: a producer's
	// automatically-extracted claims name source trees rather than packages,
	// so they land on nothing. A finding the build believes it answered then
	// comes back as noise, and nothing distinguishes that from a finding
	// nobody has looked at.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		reaches := aClaim("CVE-2026-1", sbom.AlreadyFixed, libnl, sbom.FromPedigree)
		misses := aClaim("CVE-2026-2", sbom.NotAffected,
			graph.Described{Purl: "pkg:generic/libnl3", Name: "libnl3"}, sbom.FromStatement)

		if _, err := f.store.RecordClaims(t.Context(), f.target, f.lastScan,
			[]sbom.Suppression{reaches, misses}); err != nil {
			t.Fatal(err)
		}
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)})
		if err != nil {
			t.Fatal(err)
		}
		if applied.ClaimsReaching != 1 {
			t.Errorf("%d claims reached something, want 1", applied.ClaimsReaching)
		}
		if applied.ClaimsReachingNothing != 1 {
			t.Errorf("%d claims reached nothing, want 1", applied.ClaimsReachingNothing)
		}
	})
}

func TestAnAbsurdlyLongValueDoesNotFailTheScanThatCarriedIt(t *testing.T) {
	// Everything here comes from somebody else's output. A value longer than a
	// column would fail the whole run, and a run that failed is
	// indistinguishable from a product that stopped having problems.
	each(t, func(t *testing.T, f *fixture) {
		long := strings.Repeat("x", 5000)
		f.shipped(t, graph.Snapshot{
			Root: root,
			Components: []graph.Described{{
				Purl: "pkg:deb/debian/sprawling@1.0", Name: "sprawling", Version: long,
				UpstreamName: long, UpstreamVersion: long,
			}},
			Dependencies: []graph.Dependency{{Parent: root, Child: graph.Described{
				Purl: "pkg:deb/debian/sprawling@1.0", Name: "sprawling", Version: long,
				UpstreamName: long, UpstreamVersion: long,
			}}},
		})

		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{{
			Issue: finding.Named{Identifier: "CVE-2026-1" + long, Severity: "high"},
			Component: graph.Described{
				Purl: "pkg:deb/debian/sprawling@1.0", Name: "sprawling", Version: long,
				UpstreamName: long, UpstreamVersion: long,
			},
			FixState: finding.FixedUpstream, FixedIn: long,
		}})
		if err != nil {
			t.Fatalf("a long value failed the scan carrying it: %v", err)
		}
		if applied.Opened != 1 {
			t.Errorf("opened %d findings", applied.Opened)
		}
	})
}

// holding returns a subject holding one role on the product this fixture's
// target belongs to.
func (f *fixture) holding(t *testing.T, roles ...access.Role) access.Subject {
	t.Helper()
	grants := map[int64][]access.Role{f.productID: roles}
	return access.NewPerson(1, "someone", false, grants)
}

func TestOnlyWhatSomebodyMayReadIsRead(t *testing.T) {
	// What ACC-04 is actually about. The enforcement is on the query, so this
	// tests the query rather than a handler that remembered to ask.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		// One of the two is not disclosed. Read first and then updated,
		// because one engine refuses to name the table being updated inside a
		// subquery of its own statement.
		var hidden int64
		if err := f.db.DB.NewSelect().Model((*finding.Finding)(nil)).
			ColumnExpr("MIN(id)").Scan(t.Context(), &hidden); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.DB.NewUpdate().Model((*finding.Finding)(nil)).
			Set("visibility = ?", access.Private).
			Where("id = ?", hidden).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}

		for _, c := range []struct {
			what   string
			who    access.Subject
			want   int
			denied bool
		}{
			{"public reader", f.holding(t, access.PublicRead), 1, false},
			{"public triager", f.holding(t, access.PublicTriage), 1, false},
			{"private reader", f.holding(t, access.PrivateRead), 2, false},
			{"private triager", f.holding(t, access.PrivateTriage), 2, false},
			{"an approver alone", f.holding(t), 0, true},
			{"an administrator", access.NewPerson(1, "admin", true, nil), 2, false},
			{"a pipeline", access.NewPipeline(1, "nightly", access.Scope{ProductID: f.productID}), 0, true},
		} {
			rows, err := f.store.Open(t.Context(), c.who, f.target)
			switch {
			case c.denied && !errors.Is(err, access.ErrDenied):
				t.Errorf("%s was not refused: %d rows, %v", c.what, len(rows), err)
			case c.denied:
				continue
			case err != nil:
				t.Errorf("%s: %v", c.what, err)
			case len(rows) != c.want:
				t.Errorf("%s read %d findings, want %d", c.what, len(rows), c.want)
			}

			// Counting is reading. A count of rows somebody may not see is
			// the same disclosure as the rows, compressed — and it is the
			// path that leaks when only row reads are guarded.
			n, err := f.store.CountOpen(t.Context(), c.who, f.target)
			if err != nil {
				t.Errorf("%s counting: %v", c.what, err)
				continue
			}
			if n != c.want {
				t.Errorf("%s counted %d findings, want %d", c.what, n, c.want)
			}
		}
	})
}

func TestFindingsInAProductSomebodyHoldsNothingOnAreRefused(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		// Holding everything, but on a different product.
		elsewhere := access.NewPerson(1, "elsewhere", false,
			map[int64][]access.Role{f.productID + 999: {access.PrivateRead}})

		if _, err := f.store.Open(t.Context(), elsewhere, f.target); !errors.Is(err, access.ErrDenied) {
			t.Errorf("reading another product's findings: %v", err)
		}
		if _, err := f.store.CountOpen(t.Context(), elsewhere, f.target); !errors.Is(err, access.ErrDenied) {
			t.Errorf("counting another product's findings: %v", err)
		}
	})
}

func TestTwoRunsAgainstOneTargetDoNotBothOpenTheSameFinding(t *testing.T) {
	// The queue hands different jobs to different workers by design, so two
	// runs against one target overlapping is ordinary rather than exotic.
	// Without a hold on the target, both read the same open findings, both
	// compute the same difference, and both write it — leaving two open rows
	// for one finding, which everything downstream reads as two problems and
	// which two separate triage decisions can then be made about.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		first, second := f.run(t), f.run(t)
		reported := []finding.Reported{found("CVE-2026-1", libnl)}

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, runID := range []int64{first, second} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = f.store.Apply(context.WithoutCancel(t.Context()), f.target, runID, reported)
			}()
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}

		// Two consumers pull the component in, so two findings — not four, and
		// not two of one and two of the other.
		open := f.open(t)
		if len(open) != 2 {
			t.Fatalf("%d findings are open after two overlapping runs, want 2", len(open))
		}
		seen := map[string]bool{}
		for _, row := range open {
			if seen[row.PlaceIdentity] {
				t.Errorf("two open findings for one place: %s", row.PlaceIdentity)
			}
			seen[row.PlaceIdentity] = true
		}
	})
}

func TestWhatIsStoredAboutAnIssueDoesNotDependOnWhichScanRanLast(t *testing.T) {
	// Reports disagree and arrive in an order nobody controls. Overwriting
	// makes what is stored a fact about scheduling; filling only the gap makes
	// it a fact about which report arrived first. Neither is a fact about the
	// vulnerability.
	//
	// So the worst claim anybody made wins, whichever order it arrived in —
	// and this puts the same two reports through in both orders and asserts
	// they agree.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())

		graded := func(id string, score, likelihood float64) finding.Reported {
			r := found(id, libnl)
			r.Issue.Score, r.Issue.Likelihood = score, likelihood
			return r
		}
		// One issue hears the mild report first, the other the severe one.
		for _, order := range [][]finding.Reported{
			{graded("CVE-2026-A", 4.2, 0.01), graded("CVE-2026-A", 9.1, 0.86)},
			{graded("CVE-2026-B", 9.1, 0.86), graded("CVE-2026-B", 4.2, 0.01)},
		} {
			for _, report := range order {
				if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
					[]finding.Reported{report}); err != nil {
					t.Fatal(err)
				}
			}
		}

		rising, falling := f.issueScore(t, "CVE-2026-A"), f.issueScore(t, "CVE-2026-B")
		if rising != falling {
			t.Errorf("the stored score is %d one way round and %d the other", rising, falling)
		}
		if rising != 910 {
			t.Errorf("kept %d, want 910 — the worst claim anybody made", rising)
		}
	})
}

// issueScore reads what is stored about an issue, in hundredths.
func (f *fixture) issueScore(t *testing.T, name string) int {
	t.Helper()
	var score int
	if err := f.db.DB.NewSelect().
		TableExpr("vulnerability AS v").
		ColumnExpr("COALESCE(v.score_centi, 0)").
		Where("v.identifier = ?", name).
		Scan(t.Context(), &score); err != nil {
		t.Fatal(err)
	}
	return score
}

// counter counts the statements a store actually issues.
//
// "It batches" is exactly the kind of claim that stays in a comment after the
// code stops doing it, so the test counts instead of asserting.
type counter struct{ queries atomic.Int64 }

func (c *counter) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	c.queries.Add(1)
	return ctx
}

func (c *counter) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestNamingAPageOfFindingsDoesNotCostAQueryPerRow(t *testing.T) {
	// This is the screen somebody opens first, against the largest product
	// they have. Each row used to be named by two queries of its own, so a
	// page of fifty was a hundred and one round trips and the cost grew with
	// the page instead of staying flat.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", libnl), found("CVE-2026-2", libnl),
			found("CVE-2026-3", swss), found("CVE-2026-4", teamd),
		}); err != nil {
			t.Fatal(err)
		}

		counted := &counter{}
		f.db.AddQueryHook(counted)
		groups, total, err := f.store.Groups(t.Context(), f.holding(t, access.PublicRead),
			f.target, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		issued := counted.queries.Load()

		if total != 4 || len(groups) != 4 {
			t.Fatalf("read %d of %d groups, want 4", len(groups), total)
		}
		for _, group := range groups {
			if group.Vulnerability == "" || group.Component == "" {
				t.Errorf("a row came back unnamed: %+v", group)
			}
		}
		// Which product this is, the list, the count, and one lookup per kind
		// of name. Four rows or four hundred, it is the same five statements —
		// where a query per row would already be thirteen at four rows.
		const flat = 5
		if issued > flat {
			t.Errorf("naming %d rows took %d statements, want no more than %d; "+
				"the cost is not flat", len(groups), issued, flat)
		}
	})
}

func TestABumpThatFixedNothingIsNotRecordedAsAFix(t *testing.T) {
	// A version change closes the old row and opens a new one, because
	// component identity carries the version. Nothing asked whether the issue
	// had actually gone, so a bump that resolved nothing was recorded as
	// "fixed by upgrading" — and the same issue then appeared as fixed and as
	// newly present in one release comparison, which is a document that goes
	// to customers.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		// Bumped, and the scanner still reports it: 3.9.0 is not far enough.
		f.shipped(t, movedTo(libnlNew))
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnlNew)}); err != nil {
			t.Fatal(err)
		}

		closed, opened := f.byState(t)
		if len(closed) != 2 || len(opened) != 2 {
			t.Fatalf("closed %d and opened %d, want 2 and 2", len(closed), len(opened))
		}
		for _, row := range closed {
			if row.ClosedBecause != finding.Superseded {
				t.Errorf("a bump that fixed nothing closed as %q", row.ClosedBecause)
			}
		}
		// And what it moved from is on the new row, so saying "3.7.0 → 3.9.0"
		// costs no second query.
		for _, row := range opened {
			if row.ArrivedFrom != "3.7.0" {
				t.Errorf("the new finding says it arrived from %q, want 3.7.0", row.ArrivedFrom)
			}
		}
	})
}

func TestABumpThatDidFixItStillReadsAsAFix(t *testing.T) {
	// The direction that must not break. An upgrade that actually resolved
	// something is the ordinary good case, and calling it superseded would
	// make every real fix disappear from what was fixed.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		// Bumped, and the scanner reports nothing against the new version.
		f.shipped(t, movedTo(libnlNew))
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{}); err != nil {
			t.Fatal(err)
		}

		closed, opened := f.byState(t)
		if len(opened) != 0 {
			t.Fatalf("%d findings are still open after a real fix", len(opened))
		}
		for _, row := range closed {
			if row.ClosedBecause != finding.Upgraded {
				t.Errorf("a bump that resolved it closed as %q", row.ClosedBecause)
			}
			if row.ArrivedFrom != "" {
				t.Errorf("a resolved finding was marked as having arrived from %q", row.ArrivedFrom)
			}
		}
	})
}

// movedTo is the same graph with the library at another version.
func movedTo(library graph.Described) graph.Snapshot {
	return graph.Snapshot{
		Root:       root,
		Components: []graph.Described{swss, teamd, library},
		Dependencies: []graph.Dependency{
			{Parent: root, Child: swss}, {Parent: root, Child: teamd},
			{Parent: swss, Child: library}, {Parent: teamd, Child: library},
		},
	}
}

// byState splits this target's findings into closed and open.
func (f *fixture) byState(t *testing.T) (closed, opened []finding.Finding) {
	t.Helper()
	var rows []finding.Finding
	if err := f.db.NewSelect().Model(&rows).
		Where("target_id = ?", f.target).Order("id").Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ClosedRunID != nil {
			closed = append(closed, row)
			continue
		}
		opened = append(opened, row)
	}
	return closed, opened
}

// The number a release-over-release chart is drawn from. Nothing reported it
// before, so the comparison screen could say what changed between two builds
// and not whether the estate was getting better or worse across all of them.
func TestWhatIsOpenIsReportedPerBuild(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", libnl),
			found("CVE-2026-2", libnl),
		}); err != nil {
			t.Fatal(err)
		}

		releases, err := f.store.Releases(t.Context(), f.holding(t, access.PublicRead), f.productID)
		if err != nil {
			t.Fatalf("releases: %v", err)
		}
		if len(releases) != 1 {
			t.Fatalf("reported %d builds, expected 1", len(releases))
		}
		open := f.open(t)
		if releases[0].Open != len(open) {
			t.Errorf("counted %d open, but %d findings are open — a chart that "+
				"disagrees with the list is worse than no chart",
				releases[0].Open, len(open))
		}
		// The split has to add up to the total, or the chart's segments and its
		// height are two different numbers.
		summed := 0
		for _, n := range releases[0].BySeverity {
			summed += n
		}
		if summed != releases[0].Open {
			t.Errorf("the severity split adds to %d, the total says %d", summed, releases[0].Open)
		}
		if releases[0].Stream == "" || releases[0].Variant == "" {
			t.Errorf("a build with no name: %+v", releases[0])
		}
	})
}

// A count nobody may read is not a smaller count, and a total is as much a
// disclosure as a row: "there are six here" tells a reader there are six.
func TestWhatIsOpenPerBuildIsNarrowedToWhatSomebodyMayRead(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.DB.NewUpdate().Model((*finding.Finding)(nil)).
			Set("visibility = ?", access.Private).
			Where("target_id = ?", f.target).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}

		releases, err := f.store.Releases(t.Context(), f.holding(t, access.PublicRead), f.productID)
		if err != nil {
			t.Fatalf("releases: %v", err)
		}
		for _, r := range releases {
			if r.Open != 0 {
				t.Errorf("a reader who may not see it was told there are %d", r.Open)
			}
		}
	})
}

// A build reporting nothing wrong and a build last measured against a database
// from March look identical on every screen without this.
func TestTheLastRunSaysWhatItWasMeasuredWith(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		who := f.holding(t, access.PublicRead)

		// Nothing has finished, so there is nothing to say — absent rather
		// than invented.
		before, err := f.store.LatestRun(t.Context(), who, f.target)
		if err != nil {
			t.Fatalf("latest run: %v", err)
		}
		if before != nil {
			t.Fatalf("a run was reported before any had finished: %+v", before)
		}

		f.shipped(t, twoConsumers())
		runID := f.run(t)
		if _, err := f.store.Apply(t.Context(), f.target, runID,
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		if err := f.store.Finish(t.Context(), runID, "0.100.1", "2026-08-29", nil); err != nil {
			t.Fatal(err)
		}

		last, err := f.store.LatestRun(t.Context(), who, f.target)
		if err != nil {
			t.Fatalf("latest run: %v", err)
		}
		if last == nil {
			t.Fatal("nothing reported after a run finished")
		}
		if last.Scanner != "grype" {
			t.Errorf("scanner is %q, expected grype", last.Scanner)
		}
		if last.DatabaseVersion != "2026-08-29" {
			t.Errorf("database version is %q, expected the one the run finished with",
				last.DatabaseVersion)
		}
		if last.FinishedAt == nil {
			t.Error("a finished run with no finishing time")
		}
	})
}
