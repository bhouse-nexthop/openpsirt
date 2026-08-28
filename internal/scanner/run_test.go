package scanner_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/scanner"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// stub stands in for a scanner, so what the runner does with an answer is
// testable without a scanner and its database being installed.
type stub struct {
	saw      []byte
	reported []finding.Reported
	fail     error
}

func (s *stub) Name() string { return "stub" }

func (s *stub) Scan(_ context.Context, inventory io.Reader) (scanner.Result, error) {
	s.saw, _ = io.ReadAll(inventory)
	if s.fail != nil {
		return scanner.Result{}, s.fail
	}
	return scanner.Result{
		Version: "9.9.9", DatabaseVersion: "2026-08-28",
		Reported: s.reported,
	}, nil
}

func at(name, version string) graph.Described {
	return graph.Described{
		Purl: "pkg:deb/debian/" + name + "@" + version, Name: name, Version: version,
	}
}

var (
	root  = at("sonic", "1.0")
	swss  = at("libswsscommon", "1.0.0")
	libnl = at("libnl-3-200", "3.7.0")
)

type runFixture struct {
	db     *database.DB
	queue  *queue.Queue
	target int64
}

func eachRun(t *testing.T, fn func(t *testing.T, f *runFixture)) {
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
		if _, err := cat.DeclareStream(ctx, product.ID, "master", catalog.Branch, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := cat.DeclareVariant(ctx, product.ID, "broadcom", true); err != nil {
			t.Fatal(err)
		}
		target, err := cat.Resolve(ctx, "sonic", "master", "broadcom")
		if err != nil {
			t.Fatal(err)
		}

		// What the build shipped, already read and stored.
		scan, outcome, err := ingest.NewStore(db.DB).Record(ctx, ingest.Arriving{
			TargetID: target.ID, ContentHash: "hash-1",
			BuiltAt: time.Now().UTC().Add(-time.Hour), ParserVersion: "test",
		})
		if err != nil || outcome != ingest.Accept {
			t.Fatalf("record scan: %v %v", outcome, err)
		}
		_, err = graph.NewStore(db.DB).Apply(ctx, target.ID, scan.ID, graph.Snapshot{
			Root: root, Components: []graph.Described{swss, libnl},
			Dependencies: []graph.Dependency{
				{Parent: root, Child: swss}, {Parent: swss, Child: libnl},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		fn(t, &runFixture{db: db, queue: queue.New(db, queue.DefaultOptions()), target: target.ID})
	})
}

// waiting leaves the work behind that an arriving inventory would.
func (f *runFixture) waiting(t *testing.T) {
	t.Helper()
	if _, err := f.queue.Add(t.Context(), scanner.JobKind, strconv.FormatInt(f.target, 10)); err != nil {
		t.Fatal(err)
	}
}

func TestScanningATargetProducesFindings(t *testing.T) {
	eachRun(t, func(t *testing.T, f *runFixture) {
		s := &stub{reported: []finding.Reported{{
			Issue:     finding.Named{Identifier: "CVE-2026-1", Severity: "high"},
			Component: libnl, FixState: finding.FixedUpstream, FixedIn: "3.9.0",
		}}}
		f.waiting(t)

		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		outcome, err := scanner.NewRunner(f.db, f.queue, s, quiet, "test").Once(t.Context())
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if outcome == nil {
			t.Fatal("there was work waiting and nothing was done")
		}
		if outcome.Applied.Opened != 1 {
			t.Errorf("opened %d findings, want 1", outcome.Applied.Opened)
		}

		// The scanner is given what we stored, not what a build sent — the
		// file it sent is not kept for a moving line.
		if outcome.Components != 2 {
			t.Errorf("scanned %d components, want 2", outcome.Components)
		}
		for _, want := range []string{"libnl-3-200", "libswsscommon", "pkg:deb/debian/"} {
			if !bytes.Contains(s.saw, []byte(want)) {
				t.Errorf("the scanner was not given %q", want)
			}
		}
		// The product itself is not a package any database has heard of.
		if bytes.Contains(s.saw, []byte("\"sonic\"")) {
			t.Error("the product was sent to the scanner as though it were a package")
		}
	})
}

func TestWhatRanIsRecordedAgainstTheRun(t *testing.T) {
	// A finding that appeared or vanished because the scanner or its data
	// moved is unexplainable without this.
	eachRun(t, func(t *testing.T, f *runFixture) {
		s := &stub{}
		f.waiting(t)
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		outcome, err := scanner.NewRunner(f.db, f.queue, s, quiet, "test").Once(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		var run finding.Run
		if err := f.db.DB.NewSelect().Model(&run).Where("id = ?", outcome.RunID).Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		if run.Scanner != "stub" || run.ScannerVersion != "9.9.9" || run.DatabaseVersion != "2026-08-28" {
			t.Errorf("recorded %+v", run)
		}
		if !run.RanHere {
			t.Error("a scan we ran says it came from a producer")
		}
		if run.FinishedAt == nil {
			t.Error("a run that ended is still open")
		}
	})
}

func TestAScannerThatFailedIsRecordedAsOne(t *testing.T) {
	// A scanner that stopped working is otherwise indistinguishable from a
	// product that stopped having problems.
	eachRun(t, func(t *testing.T, f *runFixture) {
		s := &stub{fail: errors.New("database is corrupt")}
		f.waiting(t)
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if _, err := scanner.NewRunner(f.db, f.queue, s, quiet, "test").Once(t.Context()); err == nil {
			t.Fatal("a failed scan reported success")
		}

		var run finding.Run
		if err := f.db.DB.NewSelect().Model(&run).Limit(1).Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		if run.Failure == "" {
			t.Error("a run that failed does not say why")
		}
		if run.FinishedAt == nil {
			t.Error("a run that failed is still open")
		}
	})
}

func TestScanningAgainWithTheSameAnswerWritesNothing(t *testing.T) {
	eachRun(t, func(t *testing.T, f *runFixture) {
		reported := []finding.Reported{{
			Issue:     finding.Named{Identifier: "CVE-2026-1", Severity: "high"},
			Component: libnl,
		}}
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		runner := scanner.NewRunner(f.db, f.queue, &stub{reported: reported}, quiet, "test")

		f.waiting(t)
		if _, err := runner.Once(t.Context()); err != nil {
			t.Fatal(err)
		}
		f.waiting(t)
		outcome, err := runner.Once(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !outcome.Applied.Unchanged() {
			t.Errorf("a re-scan finding the same things wrote %+v", outcome.Applied)
		}
	})
}

func TestThereIsNothingToScanWhenNothingIsWaiting(t *testing.T) {
	eachRun(t, func(t *testing.T, f *runFixture) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		outcome, err := scanner.NewRunner(f.db, f.queue, &stub{}, quiet, "test").Once(t.Context())
		if err != nil || outcome != nil {
			t.Errorf("an empty queue produced %+v (%v)", outcome, err)
		}
	})
}
