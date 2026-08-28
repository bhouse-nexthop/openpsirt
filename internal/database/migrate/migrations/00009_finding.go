package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/bhouse-nexthop/openpsirt/internal/database/migrate"
)

func init() {
	goose.AddMigrationContext(upFinding, downFinding)
}

// What a scan run found, and what it is about.
//
// A vulnerability is one issue however many names it goes by. The same issue
// arrives as a national identifier from one database and an advisory
// identifier from another, and which one a scanner calls primary is a
// preference of whichever source it consulted rather than a property of the
// issue. Keying anything on that choice would lapse every decision the day a
// scanner changed its mind, so the aliases resolve to one row.
//
// A finding is a vulnerability at a place: the component, and the thing that
// directly pulled it in. One issue in a shared library is one finding per
// consumer, because different consumers use different parts of what they
// depend on.
//
// Findings are held over intervals against scan runs, the same shape the graph
// uses, for the same reason: re-scanning nightly against a database that moved
// slightly must write only what changed.
func upFinding(ctx context.Context, tx *sql.Tx) error {
	e := migrate.EngineFrom(ctx)
	t := typesFor(e)
	if t == nil {
		return fmt.Errorf("no schema for %s", e)
	}

	statements := []string{
		// identity is derived from the identifier this issue is filed under
		// here, which is whichever of its names is the most widely recognized.
		`CREATE TABLE vulnerability (
			id            ` + t.id + `,
			identity      ` + t.hash + ` NOT NULL,
			identifier    ` + t.name + ` NOT NULL,
			severity      ` + t.kind + ` NULL,
			first_seen_at ` + t.timestamp + ` NOT NULL,
			CONSTRAINT vulnerability_identity_unique UNIQUE (identity)
		)` + t.suffix,

		// Every name an issue is known by, including the one it is filed
		// under. A report naming any of them finds the same row.
		`CREATE TABLE vulnerability_alias (
			id               ` + t.id + `,
			vulnerability_id ` + t.ref + ` NOT NULL REFERENCES vulnerability(id),
			identifier       ` + t.name + ` NOT NULL,
			CONSTRAINT vulnerability_alias_unique UNIQUE (identifier)
		)` + t.suffix,

		// One execution of a scanner over one variant. Recorded whether it ran
		// here or arrived from a producer, because a report that averages two
		// scanners without saying so is worse than no report.
		`CREATE TABLE scan_run (
			id               ` + t.id + `,
			target_id        ` + t.ref + ` NOT NULL REFERENCES target(id),
			scanner          ` + t.name + ` NOT NULL,
			scanner_version  ` + t.name + ` NULL,
			database_version ` + t.name + ` NULL,
			ran_here         ` + t.boolean + ` NOT NULL,
			started_at       ` + t.timestamp + ` NOT NULL,
			finished_at      ` + t.timestamp + ` NULL,
			failure          ` + t.text + ` NULL
		)` + t.suffix,

		// What a build has already argued does not apply to it.
		//
		// Stored as data rather than left in the document it arrived in. A
		// nightly scan's documents are discarded once read, the vulnerability
		// scan runs after that, and it runs again on a schedule — so a claim
		// that lived only in the file would be gone by the time anything
		// needed it, and every carried patch would come back as an
		// outstanding vulnerability on the first re-scan.
		//
		// Held over intervals against scans, like the graph: a build argues
		// the same things night after night, and re-sending them must write
		// nothing.
		`CREATE TABLE suppression (
			id             ` + t.id + `,
			target_id      ` + t.ref + ` NOT NULL REFERENCES target(id),
			identity       ` + t.hash + ` NOT NULL,
			vulnerability  ` + t.name + ` NOT NULL,
			status         ` + t.kind + ` NOT NULL,
			justification  ` + t.name + ` NULL,
			statement      ` + t.text + ` NULL,
			origin         ` + t.kind + ` NOT NULL,
			subject_purl   ` + t.text + ` NULL,
			subject_name   ` + t.name + ` NULL,
			opened_scan_id ` + t.ref + ` NOT NULL REFERENCES scan(id),
			closed_scan_id ` + t.refNull + ` NULL REFERENCES scan(id)
		)` + t.suffix,

		`CREATE INDEX suppression_open_idx ON suppression (target_id, closed_scan_id)`,

		// place_identity is the hashed pair of names — the component and its
		// consumer — which is what a triage decision is keyed on. It is stored
		// rather than derived so a decision can be found without walking the
		// graph of every variant it might apply to.
		`CREATE TABLE finding (
			id               ` + t.id + `,
			target_id        ` + t.ref + ` NOT NULL REFERENCES target(id),
			kind             ` + t.kind + ` NOT NULL,
			vulnerability_id ` + t.ref + ` NOT NULL REFERENCES vulnerability(id),
			component_id     ` + t.ref + ` NOT NULL REFERENCES component(id),
			consumer_id      ` + t.refNull + ` NULL REFERENCES component(id),
			place_identity   ` + t.hash + ` NOT NULL,
			fix_state        ` + t.kind + ` NULL,
			fixed_in         ` + t.name + ` NULL,
			-- A finding the build has already argued about is marked, not
			-- dropped: one that simply stopped appearing is
			-- indistinguishable from a scanner fault.
			suppressed_by    ` + t.refNull + ` NULL REFERENCES suppression(id),
			opened_run_id    ` + t.ref + ` NOT NULL REFERENCES scan_run(id),
			closed_run_id    ` + t.refNull + ` NULL REFERENCES scan_run(id),
			closed_because   ` + t.kind + ` NULL
		)` + t.suffix,

		// What is open now, per variant, is the query behind every screen.
		`CREATE INDEX finding_open_idx ON finding (target_id, closed_run_id)`,
		// Finding one issue everywhere it is present, which is what triaging
		// one vulnerability across a portfolio asks for.
		`CREATE INDEX finding_vulnerability_idx ON finding (vulnerability_id, closed_run_id)`,
		// Carrying a decision forward to the same place elsewhere.
		`CREATE INDEX finding_place_idx ON finding (place_identity)`,
	}

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func downFinding(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		`DROP TABLE finding`,
		`DROP TABLE suppression`,
		`DROP TABLE scan_run`,
		`DROP TABLE vulnerability_alias`,
		`DROP TABLE vulnerability`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
