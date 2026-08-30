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
		`CREATE TABLE "vulnerability" (
			"id"            ` + t.id + `,
			"identity"      ` + t.hash + ` NOT NULL,
			"identifier"    ` + t.free + ` NOT NULL,
			"severity"      ` + t.kind + ` NULL,
			-- What somebody triaging needs in front of them. There may be
			-- thousands of these and very few people, so a finding that
			-- carries its own evidence is the difference between a queue that
			-- gets worked and one that does not.
			"description"     ` + t.text + ` NULL,
			-- Where the issue is written up. Every report carries one, and for
			-- the great majority it is the only route to the patch — so it is
			-- the single most valuable thing a report gives us.
			"advisory"        ` + t.free + ` NULL,
			-- Whether somebody is known to be exploiting it, and the published
			-- estimate that they will. Together these are what separates the
			-- handful that matter from the thousands that can wait.
			"exploited"       ` + t.boolean + ` NOT NULL,
			-- Held as parts per million rather than as a fraction. Every engine
			-- spells an exact decimal differently and a float compares
			-- differently again, and this has to sort in an index.
			"likelihood_ppm"  ` + t.ref + ` NULL,
			-- The severity as a number, and the statement of what it assumes.
			-- Network-reachable and unauthenticated is a different judgment
			-- from local-and-privileged at the same number, and the vector is
			-- where that shows.
			"score_centi"     ` + t.ref + ` NULL,
			"vector"          ` + t.free + ` NULL,
			"first_seen_at" ` + t.timestamp + ` NOT NULL,
			CONSTRAINT "vulnerability_identity_unique" UNIQUE ("identity")
		)` + t.suffix,

		// Every name an issue is known by, including the one it is filed
		// under. A report naming any of them finds the same row.
		`CREATE TABLE "vulnerability_reference" (
			"id"               ` + t.id + `,
			"vulnerability_id" ` + t.ref + ` NOT NULL,
			"url"              ` + t.free + ` NOT NULL,
			-- What it appears to be, as far as the address reveals. A patch is
			-- the one worth telling apart: somebody deciding whether to
			-- backport rather than upgrade needs it, and hunting for it by
			-- hand is the step that does not happen when a thousand others are
			-- waiting.
			"kind"             ` + t.kind + ` NOT NULL,
			-- The hash of the address, because the address itself is longer
			-- than an index key may be on some engines.
			"url_identity"     ` + t.hash + ` NOT NULL,
			CONSTRAINT "vulnerability_reference_vulnerability_fk" FOREIGN KEY ("vulnerability_id") REFERENCES "vulnerability"("id"),
			CONSTRAINT "vulnerability_reference_unique" UNIQUE ("vulnerability_id", "url_identity")
		)` + t.suffix,

		`CREATE INDEX "vulnerability_reference_vulnerability_idx" ON "vulnerability_reference" ("vulnerability_id")`,

		`CREATE TABLE "vulnerability_alias" (
			"id"               ` + t.id + `,
			"vulnerability_id" ` + t.ref + ` NOT NULL,
			"identifier"       ` + t.name + ` NOT NULL,
			CONSTRAINT "vulnerability_alias_unique" UNIQUE ("identifier"),
			CONSTRAINT "vulnerability_alias_vulnerability_id_fk" FOREIGN KEY ("vulnerability_id") REFERENCES "vulnerability"("id")
		)` + t.suffix,

		// One execution of a scanner over one variant. Recorded whether it ran
		// here or arrived from a producer, because a report that averages two
		// scanners without saying so is worse than no report.
		`CREATE TABLE "scan_run" (
			"id"               ` + t.id + `,
			"target_id"        ` + t.ref + ` NOT NULL,
			"scanner"          ` + t.name + ` NOT NULL,
			"scanner_version"  ` + t.name + ` NULL,
			"database_version" ` + t.name + ` NULL,
			"ran_here"         ` + t.boolean + ` NOT NULL,
			"started_at"       ` + t.timestamp + ` NOT NULL,
			"finished_at"      ` + t.timestamp + ` NULL,
			"failure"          ` + t.text + ` NULL,
			CONSTRAINT "scan_run_target_id_fk" FOREIGN KEY ("target_id") REFERENCES "target"("id")
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
		`CREATE TABLE "suppression" (
			"id"             ` + t.id + `,
			"target_id"      ` + t.ref + ` NOT NULL,
			"identity"       ` + t.hash + ` NOT NULL,
			"vulnerability"  ` + t.free + ` NOT NULL,
			-- The status vocabulary is the exchange format's, not ours, and
			-- its longest word today is longer than a short identifier
			-- column allows. Whatever it adds next is not ours to bound.
			"status"         ` + t.name + ` NOT NULL,
			"justification"  ` + t.free + ` NULL,
			"statement"      ` + t.text + ` NULL,
			"origin"         ` + t.kind + ` NOT NULL,
			"subject_purl"   ` + t.text + ` NULL,
			"subject_name"   ` + t.free + ` NULL,
			"opened_scan_id" ` + t.ref + ` NOT NULL,
			"closed_scan_id" ` + t.refNull + ` NULL,
			CONSTRAINT "suppression_target_id_fk" FOREIGN KEY ("target_id") REFERENCES "target"("id"),
			CONSTRAINT "suppression_opened_scan_id_fk" FOREIGN KEY ("opened_scan_id") REFERENCES "scan"("id"),
			CONSTRAINT "suppression_closed_scan_id_fk" FOREIGN KEY ("closed_scan_id") REFERENCES "scan"("id")
		)` + t.suffix,

		`CREATE INDEX "suppression_open_idx" ON "suppression" ("target_id", "closed_scan_id")`,

		// place_identity is the hashed pair of names — the component and its
		// consumer — which is what a triage decision is keyed on. It is stored
		// rather than derived so a decision can be found without walking the
		// graph of every variant it might apply to.
		`CREATE TABLE "finding" (
			"id"               ` + t.id + `,
			"target_id"        ` + t.ref + ` NOT NULL,
			"kind"             ` + t.kind + ` NOT NULL,
			-- Whether this has been disclosed. Not who may read it: every
			-- request is authenticated either way. Anything unrecognized
			-- reads as undisclosed, so a value added later cannot default
			-- rows that predate it to visible.
			"visibility"       ` + t.kind + ` NOT NULL,
			"vulnerability_id" ` + t.ref + ` NOT NULL,
			"component_id"     ` + t.ref + ` NOT NULL,
			"consumer_id"      ` + t.refNull + ` NULL,
			"place_identity"   ` + t.hash + ` NOT NULL,
			"fix_state"        ` + t.kind + ` NULL,
			-- A scanner reports every version that fixes an issue, and for a
			-- kernel that is a long list.
			"fixed_in"         ` + t.free + ` NULL,
			-- When the fixing version became available. "Fixed upstream
			-- fourteen months ago" is a different conversation from "fixed in
			-- 0.17.0", and it is the one that says whether an upgrade is
			-- overdue or fresh.
			"fixed_at"       ` + t.date + ` NULL,
			-- A finding the build has already argued about is marked, not
			-- dropped: one that simply stopped appearing is
			-- indistinguishable from a scanner fault.
			"suppressed_by"    ` + t.refNull + ` NULL,
			-- What is true of a finding now, alongside when it became true.
			-- A finding open for years outlives whatever record of the change
			-- was kept elsewhere, so it carries its own.
			-- How urgent this is, as one number that sorts. Written when a scan
			-- is applied rather than worked out while reading: sorting tens of
			-- thousands of rows has to hit an index, and a number computed on
			-- read cannot be one.
			--
			-- What it was made of is kept beside it, so a position can be
			-- explained. Reading the signals back out of the packed number
			-- would work and would break silently the first time the weighting
			-- changed.
			-- Named urgency rather than rank because rank is a reserved word
			-- on one of the four engines, which accepted it everywhere else
			-- and refused the table outright there.
			"urgency"           ` + t.ref + ` NOT NULL,
			"urgency_exploited" ` + t.boolean + ` NOT NULL,
			"urgency_shipped"   ` + t.boolean + ` NOT NULL,
			"last_changed_at"  ` + t.timestamp + ` NOT NULL,
			"opened_run_id"    ` + t.ref + ` NOT NULL,
			"closed_run_id"    ` + t.refNull + ` NULL,
			"closed_because"   ` + t.kind + ` NULL,
			CONSTRAINT "finding_target_id_fk" FOREIGN KEY ("target_id") REFERENCES "target"("id"),
			CONSTRAINT "finding_vulnerability_id_fk" FOREIGN KEY ("vulnerability_id") REFERENCES "vulnerability"("id"),
			CONSTRAINT "finding_component_id_fk" FOREIGN KEY ("component_id") REFERENCES "component"("id"),
			CONSTRAINT "finding_consumer_id_fk" FOREIGN KEY ("consumer_id") REFERENCES "component"("id"),
			CONSTRAINT "finding_suppressed_by_fk" FOREIGN KEY ("suppressed_by") REFERENCES "suppression"("id"),
			CONSTRAINT "finding_opened_run_id_fk" FOREIGN KEY ("opened_run_id") REFERENCES "scan_run"("id"),
			CONSTRAINT "finding_closed_run_id_fk" FOREIGN KEY ("closed_run_id") REFERENCES "scan_run"("id")
		)` + t.suffix,

		// What is open now, per variant, is the query behind every screen.
		`CREATE INDEX "finding_open_idx" ON "finding" ("target_id", "closed_run_id")`,
		// Finding one issue everywhere it is present, which is what triaging
		// one vulnerability across a portfolio asks for.
		`CREATE INDEX "finding_vulnerability_idx" ON "finding" ("vulnerability_id", "closed_run_id")`,
		// Carrying a decision forward to the same place elsewhere.
		`CREATE INDEX "finding_place_idx" ON "finding" ("place_identity")`,
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
		`DROP TABLE "finding"`,
		`DROP TABLE "suppression"`,
		`DROP TABLE "scan_run"`,
		`DROP TABLE "vulnerability_reference"`,
		`DROP TABLE "vulnerability_alias"`,
		`DROP TABLE "vulnerability"`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
