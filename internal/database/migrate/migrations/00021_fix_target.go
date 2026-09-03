package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/bhouse-nexthop/openpsirt/internal/database/migrate"
)

func init() {
	goose.AddMigrationContext(upFixTarget, downFixTarget)
}

// Which builds somebody intends to fix an issue in.
//
// Declared intent, not commits. Nothing here watches a repository, so what can
// be recorded is which releases the fix is meant to reach; whether it arrived
// is answered by the next scan of each of them rather than by anybody saying
// so (REM-07, REM-09).
//
// **The row is the declaration and nothing else.** There is no state column,
// no "done", no resolved-at. An issue is fixed in a build when the build stops
// holding it, which the findings already say — a second record of the same
// fact would be one somebody has to keep true, and the way that fails is the
// tool reporting a fix that shipped in nobody's release.
//
// **Keyed on the piece of work rather than on a finding.** An issue in a
// component is one thing to fix however many places it sits at and however
// many variants ship it, which is the unit assignment already uses (REL-01,
// REM-10). The build is the target, so the key is that trio plus the build.
//
// The product is not a column. It is reached through the build, like every
// other query here, and storing it beside a build that already answers it is a
// second copy of one fact.
func upFixTarget(ctx context.Context, tx *sql.Tx) error {
	e := migrate.EngineFrom(ctx)
	t := typesFor(e)
	if t == nil {
		return fmt.Errorf("no schema for %s", e)
	}

	statements := []string{
		`CREATE TABLE "fix_target" (
			"id"               ` + t.id + `,
			"vulnerability_id" ` + t.ref + ` NOT NULL,
			"component_id"     ` + t.ref + ` NOT NULL,
			"target_id"        ` + t.ref + ` NOT NULL,
			"declared_by"      ` + t.ref + ` NOT NULL,
			"declared_at"      ` + t.timestamp + ` NOT NULL,
			CONSTRAINT "fix_target_unique" UNIQUE ("vulnerability_id", "component_id", "target_id"),
			CONSTRAINT "fix_target_vulnerability_id_fk" FOREIGN KEY ("vulnerability_id") REFERENCES "vulnerability"("id"),
			CONSTRAINT "fix_target_component_id_fk" FOREIGN KEY ("component_id") REFERENCES "component"("id"),
			CONSTRAINT "fix_target_target_id_fk" FOREIGN KEY ("target_id") REFERENCES "target"("id"),
			CONSTRAINT "fix_target_declared_by_fk" FOREIGN KEY ("declared_by") REFERENCES "person"("id")
		)` + t.suffix,

		// What is declared for this build. Reading a build's remediation
		// intent is how a release answers "what is meant to be fixed here",
		// and without this it is a scan of every declaration ever made.
		`CREATE INDEX "fix_target_build_idx" ON "fix_target" ("target_id")`,
	}

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func downFixTarget(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{`DROP TABLE "fix_target"`} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
