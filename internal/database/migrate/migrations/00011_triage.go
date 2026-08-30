package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/bhouse-nexthop/openpsirt/internal/database/migrate"
)

func init() {
	goose.AddMigrationContext(upTriage, downTriage)
}

// What people decide about findings.
//
// The key is the whole design. A decision is a claim about a combination of
// code rather than about the release it was made in, so it is keyed on what
// that combination is: the product, the issue, the place, and the upstream
// versions of the component and of the thing that pulls it in. Nothing in the
// key names a release, which is what makes a later release inherit a decision
// by looking it up rather than by copying it — no syncing, and nothing to
// drift.
//
// The same key is what makes expiry work without a mechanism of its own. When
// a version moves, the key a finding computes stops matching the key a
// decision was stored under, and the decision simply no longer applies. That
// is why identity here is structural and expiry is version-based and neither
// borrows from the other (MDL-08): overlapping them is how a bump at the top
// of a build invalidates a judgment made about a leaf.
func upTriage(ctx context.Context, tx *sql.Tx) error {
	e := migrate.EngineFrom(ctx)
	t := typesFor(e)
	if t == nil {
		return fmt.Errorf("no schema for %s", e)
	}

	statements := []string{
		// One claim about one combination of code.
		//
		// place_identity is the structural half — the hashed pair of names,
		// no versions anywhere in it (MDL-06). The two version columns are the
		// expiry half. Keeping them in separate columns rather than folded
		// into one key is what lets the structural half be looked up on its
		// own, which is how a decision that has lapsed is found again in order
		// to offer its reasoning back to whoever has to re-make it.
		//
		// consumer_upstream_version is absent where the thing above is the
		// product itself, which is excluded from identity and from expiry
		// because its version changes on every build (MDL-07).
		`CREATE TABLE "decision" (
			"id"                         ` + t.id + `,
			"product_id"                 ` + t.ref + ` NOT NULL,
			"vulnerability_id"           ` + t.ref + ` NOT NULL,
			"place_identity"             ` + t.hash + ` NOT NULL,
			-- The finding's, carried here so that who may reach a decision is
			-- answered by the row rather than by whoever built the query. A
			-- finding nobody has disclosed is one only a private triager may
			-- argue about, and that has to hold for a report and an export as
			-- much as for reading one.
			"visibility"                 ` + t.kind + ` NOT NULL,
			"component_upstream_version" ` + t.name + ` NULL,
			"consumer_upstream_version"  ` + t.name + ` NULL,
			"outcome"                    ` + t.kind + ` NOT NULL,
			-- Only "not applicable" carries one, and it is required there:
			-- the claim is that something does not affect us, and which of the
			-- recognized reasons it is *is* the claim.
			"justification"              ` + t.free + ` NULL,
			-- Set only for a deferral, which is the one outcome that expires
			-- on a date rather than on the code changing.
			"deferred_until"             ` + t.date + ` NULL,
			-- How bad this was judged to be when the claim was made, in
			-- hundredths. Kept with the decision rather than read from the
			-- issue later, because the question a re-affirmation asks is
			-- whether severity has risen *since* — and an issue's severity is
			-- rewritten in place as reports revise it, so reading it now would
			-- always compare a number against itself.
			"severity_centi"             ` + t.ref + ` NULL,
			-- Whether a second person has to agree before this takes effect.
			-- Recorded rather than worked out on each read: most outcomes need
			-- agreement and a short deferral does not, and without somewhere
			-- to keep that, "proposed" would have to mean "in force" for
			-- everything — which makes the review queue decorative.
			"needs_approval"             ` + t.boolean + ` NOT NULL,
			"state"                      ` + t.kind + ` NOT NULL,
			"proposed_by"                ` + t.ref + ` NOT NULL,
			"proposed_at"                ` + t.timestamp + ` NOT NULL,
			-- What the current reasoning is. An approval points at one
			-- revision rather than at the decision, so this moving is exactly
			-- what withdraws an approval.
			"revision_id"                ` + t.refNull + ` NULL,
			CONSTRAINT "decision_product_fk" FOREIGN KEY ("product_id") REFERENCES "product"("id"),
			CONSTRAINT "decision_vulnerability_fk" FOREIGN KEY ("vulnerability_id") REFERENCES "vulnerability"("id"),
			CONSTRAINT "decision_proposer_fk" FOREIGN KEY ("proposed_by") REFERENCES "person"("id")
		)` + t.suffix,

		// Found two ways, and both are hot.
		//
		// The first is exact: a finding asks whether a decision applies to it,
		// which is every version column matching as well as the structural
		// ones. The second drops the versions, and answers "was there ever a
		// decision about this place" — which is what surfaces the reasoning
		// behind one that has lapsed, so somebody re-making it is not starting
		// from a blank page.
		`CREATE INDEX "decision_applies_idx" ON "decision" ("product_id", "vulnerability_id", "place_identity", "component_upstream_version", "consumer_upstream_version")`,
		`CREATE INDEX "decision_place_idx" ON "decision" ("product_id", "vulnerability_id", "place_identity")`,

		// The reasoning, revised and never overwritten.
		//
		// A second person approves specific words. If those words can change
		// afterwards, the second pair of eyes reviewed something that is no
		// longer what stands — which is the whole control gone, silently. So
		// every revision is kept and readable, and an approval names one.
		`CREATE TABLE "decision_revision" (
			"id"          ` + t.id + `,
			"decision_id" ` + t.ref + ` NOT NULL,
			"ordinal"     ` + t.ref + ` NOT NULL,
			-- The text as somebody wrote it. Stored as source and never as
			-- rendered output: what is safe to render is a decision made when
			-- it is rendered, and text stored years ago predates rules written
			-- since.
			"body"        ` + t.text + ` NOT NULL,
			"written_by"  ` + t.ref + ` NOT NULL,
			"written_at"  ` + t.timestamp + ` NOT NULL,
			CONSTRAINT "decision_revision_decision_fk" FOREIGN KEY ("decision_id") REFERENCES "decision"("id"),
			CONSTRAINT "decision_revision_author_fk" FOREIGN KEY ("written_by") REFERENCES "person"("id"),
			CONSTRAINT "decision_revision_unique" UNIQUE ("decision_id", "ordinal")
		)` + t.suffix,

		`CREATE INDEX "decision_revision_decision_idx" ON "decision_revision" ("decision_id")`,

		// Who agreed, and to what exactly.
		//
		// Kept rather than reduced to a flag on the decision, because an
		// approval that was later withdrawn is part of the record: it says a
		// second person did once agree, and to which words.
		`CREATE TABLE "decision_approval" (
			"id"           ` + t.id + `,
			"decision_id"  ` + t.ref + ` NOT NULL,
			"revision_id"  ` + t.ref + ` NOT NULL,
			"approved_by"  ` + t.ref + ` NOT NULL,
			"approved_at"  ` + t.timestamp + ` NOT NULL,
			-- Set when the reasoning was revised under it, or when somebody
			-- took the decision back.
			"withdrawn_at" ` + t.timestamp + ` NULL,
			-- What a bulk approval was, so undoing one is undoing a batch
			-- rather than hunting for what it touched.
			"batch"        ` + t.hash + ` NULL,
			CONSTRAINT "decision_approval_decision_fk" FOREIGN KEY ("decision_id") REFERENCES "decision"("id"),
			CONSTRAINT "decision_approval_revision_fk" FOREIGN KEY ("revision_id") REFERENCES "decision_revision"("id"),
			CONSTRAINT "decision_approval_approver_fk" FOREIGN KEY ("approved_by") REFERENCES "person"("id")
		)` + t.suffix,

		`CREATE INDEX "decision_approval_decision_idx" ON "decision_approval" ("decision_id")`,
		`CREATE INDEX "decision_approval_batch_idx" ON "decision_approval" ("batch")`,

		// Discussion, which is not the reasoning.
		//
		// The obvious mistake is treating all text on a finding as one thing.
		// Annotating an approved decision months later — "re-checked, still
		// true" — is ordinary, and it must not disturb the approval, because
		// an approval that fell over every time somebody added a note would
		// teach people not to add notes.
		//
		// A comment is overwritten when its author edits it, and no history is
		// kept beyond a marker that it happened. That is a deliberate
		// exception to keeping everything: discussion is not the record a
		// decision rests on, and that record is the revisions and the
		// approvals, which are kept in full.
		`CREATE TABLE "decision_comment" (
			"id"          ` + t.id + `,
			"decision_id" ` + t.ref + ` NOT NULL,
			"body"        ` + t.text + ` NOT NULL,
			"written_by"  ` + t.ref + ` NOT NULL,
			"written_at"  ` + t.timestamp + ` NOT NULL,
			"edited_at"   ` + t.timestamp + ` NULL,
			CONSTRAINT "decision_comment_decision_fk" FOREIGN KEY ("decision_id") REFERENCES "decision"("id"),
			CONSTRAINT "decision_comment_author_fk" FOREIGN KEY ("written_by") REFERENCES "person"("id")
		)` + t.suffix,

		`CREATE INDEX "decision_comment_decision_idx" ON "decision_comment" ("decision_id")`,
	}

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func downTriage(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		`DROP TABLE "decision_comment"`,
		`DROP TABLE "decision_approval"`,
		`DROP TABLE "decision_revision"`,
		`DROP TABLE "decision"`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
