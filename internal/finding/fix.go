package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// FixTarget is one build somebody said they intend to fix an issue in.
//
// The row is the declaration and nothing else. Whether the fix arrived is
// answered by the next scan of that build rather than by anything stored here
// (REM-09) — a second record of the same fact is one somebody has to keep
// true, and the way that fails is the tool reporting a fix that shipped in
// nobody's release.
type FixTarget struct {
	bun.BaseModel `bun:"table:fix_target,alias:ft"`

	ID              int64     `bun:"id,pk,autoincrement"`
	VulnerabilityID int64     `bun:"vulnerability_id,notnull"`
	ComponentID     int64     `bun:"component_id,notnull"`
	TargetID        int64     `bun:"target_id,notnull"`
	DeclaredBy      int64     `bun:"declared_by,notnull"`
	DeclaredAt      time.Time `bun:"declared_at,notnull"`
}

// Intent is one build's part in fixing an issue: whether it was chosen, and
// what has happened there since.
//
// Every build of the product that holds the issue is here, chosen or not,
// because "nobody has said" is a different answer from "chosen and not done"
// and reporting them alike is how the second gets lost among the first
// (REM-13). A build that was chosen and no longer holds the issue is here too:
// that is the one that worked, and dropping it from the list would leave a
// finished piece of work looking like one nobody ever declared.
type Intent struct {
	TargetID int64
	Stream   string
	Variant  string
	// Places is how many findings of this issue the build still holds. Zero
	// where it holds none.
	Places int
	// WasHere says the build has held this issue at some point. With Places at
	// zero it is the answer somebody is looking for — it was here and it is
	// gone — and a build that never shipped the component has neither.
	WasHere bool
	// Declared says somebody chose this build, and by whom and when.
	Declared   bool
	DeclaredBy int64
	DeclaredAt *time.Time
	// ScannedSince says a scan of this build has finished since it was chosen.
	// Without one, a build still holding the issue says nothing about the fix
	// — nobody has looked yet.
	ScannedSince bool
	// PastEndOfLife says the build is out of support. Nothing on it is
	// remediated, so it carries no target and is not counted as outstanding
	// (REM-16).
	PastEndOfLife bool
}

// Counts says this build is part of the plan being measured.
//
// A release that has gone out of support is not, whatever was said about it
// before it did. Nothing on it will be fixed (REM-16), so counting it as
// outstanding fills the figure permanently and counting it as delivered claims
// a fix nobody shipped. It is neither, and it is still listed — as retired, so
// somebody who chose it before the date can see what became of it.
func (i Intent) Counts() bool { return i.Declared && !i.PastEndOfLife }

// Clear says the build was chosen and no longer holds the issue.
func (i Intent) Clear() bool { return i.Counts() && i.Places == 0 }

// Missed says the build was chosen, has been scanned since, and still holds
// the issue.
//
// The scan is independent evidence against the claim (REM-03). Without the
// "since" it would flag every declaration made between two nights, which is
// most of them and would make the flag worthless within a week.
func (i Intent) Missed() bool { return i.Counts() && i.Places > 0 && i.ScannedSince }

// Gone says the build held this issue, no longer does, and nobody planned it.
//
// The other half of "gone from main, still present in 2.4 and 2.3" (REM-06).
// Derived only from scans: nobody claimed it, and a build that fixed something
// on the way past is still a build that fixed it.
func (i Intent) Gone() bool {
	return !i.Declared && i.Places == 0 && i.WasHere
}

// Undecided says the build holds the issue and nobody has said whether it will
// be fixed there.
//
// Distinct from a build somebody chose and has not delivered yet. Nobody is
// made to answer the same question for six releases, so silence is allowed —
// but it has to read as silence rather than as a plan (REM-13).
func (i Intent) Undecided() bool {
	return !i.Declared && i.Places > 0 && !i.PastEndOfLife
}

// FixingIn reports every build's part in fixing one issue in one component.
//
// The candidate list is where the issue currently appears, which is what
// somebody choosing has in front of them (REM-07), plus every build that once
// held it and no longer does — which is the other half of the same question:
// "gone from main, still present in 2.4 and 2.3" (REM-06).
//
// Listing only where it still is would leave a build that was fixed missing
// from the list entirely, which reads identically to a build that never
// shipped the component. Those are opposite answers, and the first is the one
// somebody is looking for.
func (s *Store) FixingIn(ctx context.Context, subject access.Subject,
	productID, vulnerabilityID, componentID int64) ([]Intent, error) {

	if !subject.Sees(productID) {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	// Where it still is, per build.
	var open []struct {
		TargetID int64 `bun:"target_id"`
		Places   int   `bun:"places"`
	}
	err := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("f.target_id AS target_id").
		ColumnExpr("COUNT(*) AS places").
		Where("st.product_id = ?", productID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.component_id = ?", componentID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.target_id").
		Scan(ctx, &open)
	if err != nil {
		return nil, fmt.Errorf("read where the issue still is: %w", err)
	}

	// And where it ever was. A second pass rather than one query counting both
	// with a conditional sum: that sum comes back as a decimal on two of the
	// four engines, and the cast to make it an integer is engine-specific
	// spelling the core does not carry (DAT-02).
	var ever []int64
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("f.target_id").
		Where("st.product_id = ?", productID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.component_id = ?", componentID).
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.target_id").
		Scan(ctx, &ever)
	if err != nil {
		return nil, fmt.Errorf("read where the issue has ever been: %w", err)
	}

	declared, err := s.declaredFor(ctx, productID, vulnerabilityID, componentID)
	if err != nil {
		return nil, err
	}

	places := map[int64]int{}
	for _, row := range open {
		places[row.TargetID] = row.Places
	}
	everHeld := map[int64]bool{}
	ids := make([]int64, 0, len(ever)+len(declared))
	for _, id := range ever {
		everHeld[id] = true
		ids = append(ids, id)
	}
	for id := range declared {
		if !everHeld[id] {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	named, err := s.buildsNamed(ctx, ids)
	if err != nil {
		return nil, err
	}
	scanned, err := s.lastFinished(ctx, ids)
	if err != nil {
		return nil, err
	}
	retired, err := s.retired(ctx, ids)
	if err != nil {
		return nil, err
	}

	intents := make([]Intent, 0, len(ids))
	for _, id := range ids {
		build := named[id]
		intent := Intent{
			TargetID: id, Stream: build.Stream, Variant: build.Variant,
			Places: places[id], WasHere: everHeld[id], PastEndOfLife: retired[id],
		}
		if row, ok := declared[id]; ok {
			at := row.DeclaredAt
			intent.Declared = true
			intent.DeclaredBy = row.DeclaredBy
			intent.DeclaredAt = &at
			if finished, ok := scanned[id]; ok {
				intent.ScannedSince = finished.After(at)
			}
		}
		intents = append(intents, intent)
	}
	sortIntents(intents)
	return intents, nil
}

// FixIn replaces the set of builds an issue is to be fixed in.
//
// A set rather than a field, written whole rather than one at a time: intent
// spans several releases and is decided in one sitting, so the honest write is
// the one that says what the answer now is (REM-08). Sending an empty set is
// how a plan is withdrawn.
//
// Bounded by the catalog rather than by the request: what may be named is a
// build of this product, so the largest write anybody can ask for is one row
// per build that exists (TRI-35).
func (s *Store) FixIn(ctx context.Context, subject access.Subject,
	productID, vulnerabilityID, componentID int64, builds []int64) (int, error) {

	// Deciding what will be fixed is triage work, and it is a write, so it
	// asks for the right that names it rather than for the right to read.
	if !subject.Holds(access.PublicTriage, productID) &&
		!subject.Holds(access.PrivateTriage, productID) {
		return 0, access.Denied(fmt.Sprintf("decide what is fixed in product %d", productID))
	}
	if len(visibleTo(subject, productID)) == 0 {
		return 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	if subject.ID == 0 {
		return 0, access.Denied("declare a fix without being anybody")
	}

	wanted, err := s.buildsOf(ctx, productID, builds)
	if err != nil {
		return 0, err
	}
	retired, err := s.retired(ctx, builds)
	if err != nil {
		return 0, err
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	var declared int
	err = database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		declared = 0
		// Everything that is no longer wanted goes. Withdrawing intent is not
		// a different kind of act from declaring it, and making it one
		// produces two paths that drift.
		remove := tx.NewDelete().Model((*FixTarget)(nil)).
			Where("vulnerability_id = ?", vulnerabilityID).
			Where("component_id = ?", componentID).
			Where(`target_id IN (SELECT tg.id FROM "target" AS tg
				JOIN "stream" AS st ON st.id = tg.stream_id
				WHERE st.product_id = ?)`, productID)
		if len(wanted) > 0 {
			remove = remove.Where("target_id NOT IN (?)", bun.List(wanted))
		}
		if _, err := remove.Exec(ctx); err != nil {
			return fmt.Errorf("withdraw what is no longer intended: %w", err)
		}

		// What is already declared, read inside the transaction. A retry
		// re-runs this closure against a database that has moved, so a set
		// read before it began describes a world that is gone (DAT-31) — and
		// reading it here rather than upserting keeps the statement portable:
		// "on conflict do nothing" is two different spellings across the four
		// engines, and neither belongs in the core (DAT-02).
		var already []int64
		if err := tx.NewSelect().
			TableExpr("fix_target AS ft").
			ColumnExpr("ft.target_id").
			Where("ft.vulnerability_id = ?", vulnerabilityID).
			Where("ft.component_id = ?", componentID).
			Where(`ft.target_id IN (SELECT tg.id FROM "target" AS tg
				JOIN "stream" AS st ON st.id = tg.stream_id
				WHERE st.product_id = ?)`, productID).
			Scan(ctx, &already); err != nil {
			return fmt.Errorf("read what is already intended: %w", err)
		}
		have := make(map[int64]bool, len(already))
		for _, id := range already {
			have[id] = true
		}

		for _, id := range wanted {
			if retired[id] {
				// A release past end-of-life will not be fixed, so it cannot
				// be a target. Refused rather than accepted and ignored:
				// silently dropping it leaves somebody believing a release
				// is covered.
				return access.Denied(fmt.Sprintf(
					"declare a fix in build %d, which is out of support", id))
			}
			// Declaring what is already declared keeps the first declaration.
			// When somebody said they would fix this is a fact about a moment,
			// and rewriting the set to add one release would move every date
			// in it to today.
			if have[id] {
				continue
			}
			row := &FixTarget{
				VulnerabilityID: vulnerabilityID, ComponentID: componentID,
				TargetID: id, DeclaredBy: subject.ID, DeclaredAt: now,
			}
			if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
				return fmt.Errorf("declare the fix: %w", err)
			}
			declared++
		}
		return nil
	})
	return declared, err
}

// declaredFor reads the declarations for one piece of work.
func (s *Store) declaredFor(ctx context.Context, productID, vulnerabilityID,
	componentID int64) (map[int64]FixTarget, error) {

	var rows []FixTarget
	err := s.db.NewSelect().Model(&rows).
		Where("vulnerability_id = ?", vulnerabilityID).
		Where("component_id = ?", componentID).
		Where(`target_id IN (SELECT tg.id FROM "target" AS tg
			JOIN "stream" AS st ON st.id = tg.stream_id
			WHERE st.product_id = ?)`, productID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read what is intended to be fixed: %w", err)
	}
	declared := make(map[int64]FixTarget, len(rows))
	for _, row := range rows {
		declared[row.TargetID] = row
	}
	return declared, nil
}

// buildsOf narrows a list of builds to the ones belonging to this product.
//
// A build of somebody else's product is refused rather than dropped: naming
// one is either a mistake worth reporting or an attempt to write across a
// boundary, and both want the same answer.
func (s *Store) buildsOf(ctx context.Context, productID int64, builds []int64) ([]int64, error) {
	if len(builds) == 0 {
		return nil, nil
	}
	var here []int64
	err := s.db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("tg.id").
		Where("st.product_id = ?", productID).
		Where("tg.id IN (?)", bun.List(builds)).
		OrderExpr("tg.id").
		Scan(ctx, &here)
	if err != nil {
		return nil, fmt.Errorf("check the builds named are this product's: %w", err)
	}
	if len(here) != len(builds) {
		return nil, access.Denied("declare a fix in a build of another product")
	}
	return here, nil
}

// named is what a build is called.
type named struct {
	Stream  string `bun:"stream"`
	Variant string `bun:"variant"`
}

// buildsNamed reads what each build is called.
func (s *Store) buildsNamed(ctx context.Context, ids []int64) (map[int64]named, error) {
	out := map[int64]named{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID      int64  `bun:"id"`
		Stream  string `bun:"stream"`
		Variant string `bun:"variant"`
	}
	err := s.db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		ColumnExpr("tg.id AS id").
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		Where("tg.id IN (?)", bun.List(ids)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read what these builds are called: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = named{Stream: row.Stream, Variant: row.Variant}
	}
	return out, nil
}

// lastFinished reads when each build's most recent scan finished.
//
// A run that has not finished says nothing: it may still be about to report
// the issue. Only a completed one is evidence.
func (s *Store) lastFinished(ctx context.Context, ids []int64) (map[int64]time.Time, error) {
	out := map[int64]time.Time{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		TargetID int64     `bun:"target_id"`
		Finished time.Time `bun:"finished"`
	}
	err := s.db.NewSelect().
		TableExpr("scan_run AS sr").
		ColumnExpr("sr.target_id AS target_id").
		ColumnExpr("MAX(sr.finished_at) AS finished").
		Where("sr.target_id IN (?)", bun.List(ids)).
		Where("sr.finished_at IS NOT NULL").
		GroupExpr("sr.target_id").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read when these builds were last scanned: %w", err)
	}
	for _, row := range rows {
		out[row.TargetID] = row.Finished
	}
	return out, nil
}

// retired says which of these builds are out of support.
func (s *Store) retired(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	past, err := catalog.NewStore(s.db).StreamsPastEndOfLife(ctx, s.now().UTC())
	if err != nil {
		return nil, err
	}
	if len(past) == 0 {
		return out, nil
	}
	var retired []int64
	err = s.db.NewSelect().
		TableExpr("target AS tg").
		ColumnExpr("tg.id").
		Where("tg.id IN (?)", bun.List(ids)).
		Where("tg.stream_id IN (?)", bun.List(past)).
		Scan(ctx, &retired)
	if err != nil {
		return nil, fmt.Errorf("read which of these builds are out of support: %w", err)
	}
	for _, id := range retired {
		out[id] = true
	}
	return out, nil
}

// sortIntents puts the list in the order somebody reads it: what is wrong
// first, then what is waiting, then what is done.
func sortIntents(intents []Intent) {
	rank := func(i Intent) int {
		switch {
		case i.Missed():
			return 0
		case i.Counts() && i.Places > 0:
			return 1
		case i.Undecided():
			return 2
		case i.Clear():
			return 3
		case i.Gone():
			return 4
		default:
			return 5
		}
	}
	for i := 1; i < len(intents); i++ {
		for j := i; j > 0; j-- {
			a, b := intents[j-1], intents[j]
			if rank(a) < rank(b) ||
				(rank(a) == rank(b) && (a.Stream < b.Stream ||
					(a.Stream == b.Stream && a.Variant <= b.Variant))) {
				break
			}
			intents[j-1], intents[j] = b, a
		}
	}
}
