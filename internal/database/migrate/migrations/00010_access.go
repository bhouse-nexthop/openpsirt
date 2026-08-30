package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/bhouse-nexthop/openpsirt/internal/database/migrate"
)

func init() {
	goose.AddMigrationContext(upAccess, downAccess)
}

// Who may do what.
//
// A person exists because somebody granted them access, never because they
// managed to authenticate. Authenticating proves who someone is and says
// nothing about whether they should be here, so no sign-in path creates an
// account (ACC-21) and the first person to arrive gains nothing by being first
// (ACC-30).
//
// Admin is a property of the person rather than a grant against a product,
// because it is the one role that is global (ACC-07). Modeling it as a grant
// would mean a row whose product is absent, and a uniqueness rule over a
// column that may be absent behaves differently on each of the four engines.
func upAccess(ctx context.Context, tx *sql.Tx) error {
	e := migrate.EngineFrom(ctx)
	t := typesFor(e)
	if t == nil {
		return fmt.Errorf("no schema for %s", e)
	}

	statements := []string{
		// identity is what the sign-in path says the person is — a username
		// from a trusted header, or a subject from a provider. It is compared
		// exactly, which is why the collation is pinned.
		`CREATE TABLE "person" (
			"id"           ` + t.id + `,
			"identity"     ` + t.name + ` NOT NULL,
			"display_name" ` + t.free + ` NULL,
			"is_admin"     ` + t.boolean + ` NOT NULL,
			-- is_bootstrap is set from configuration at every startup and is
			-- kept apart from is_admin so the two cannot overwrite each other.
			-- In group-bound mode admin is re-derived from group membership at
			-- every sign-in, and without this that derivation would strip the
			-- administrator named in configuration — who is the documented way
			-- back in when the group mapping is wrong (ACC-29, ACC-32).
			"is_bootstrap" ` + t.boolean + ` NOT NULL,
			-- admin_derived says a group granted administration rather than a
			-- person. Only what a group gave is taken back when groups stop
			-- deciding, so somebody promoted inside the application survives a
			-- change of mode rather than losing access nothing can restore.
			"admin_derived" ` + t.boolean + ` NOT NULL,
			"created_at"   ` + t.timestamp + ` NOT NULL,
			"last_seen_at" ` + t.timestamp + ` NULL,
			CONSTRAINT "person_identity_unique" UNIQUE ("identity")
		)` + t.suffix,

		// A role is held against one product. What somebody may reach is the
		// union of their grants, and a product they hold no grant on is not
		// merely unreadable but invisible (ACC-08).
		`CREATE TABLE "role_grant" (
			"id"         ` + t.id + `,
			"person_id"  ` + t.ref + ` NOT NULL,
			"product_id" ` + t.ref + ` NOT NULL,
			"role"       ` + t.kind + ` NOT NULL,
			-- source says where a grant came from. A grant derived from group
			-- membership is replaced wholesale at each sign-in, so losing the
			-- group loses the role (ACC-22); one assigned by an administrator
			-- is not touched by a sign-in at all.
			"source"     ` + t.kind + ` NOT NULL,
			-- active is what makes switching role-assignment modes reversible.
			-- Turning on group-bound mode marks the assignments an
			-- administrator made inactive rather than deleting them, so
			-- switching back restores them instead of asking somebody to
			-- reconstruct them from memory (ACC-36). An inactive row grants
			-- nothing and is never counted as access (ACC-37).
			"active"     ` + t.boolean + ` NOT NULL,
			"created_at" ` + t.timestamp + ` NOT NULL,
			-- The source is part of what makes a grant one grant. Somebody can
			-- hold the same role on the same product from both sides at once:
			-- an assignment set aside when group-bound mode was turned on, and
			-- a live one derived from a group that happens to grant the same
			-- thing. Keying without the source forbids that pair, which is the
			-- pair ACC-36 exists to keep.
			CONSTRAINT "role_grant_person_fk" FOREIGN KEY ("person_id") REFERENCES "person"("id"),
			CONSTRAINT "role_grant_product_fk" FOREIGN KEY ("product_id") REFERENCES "product"("id"),
			CONSTRAINT "role_grant_unique" UNIQUE ("person_id", "product_id", "role", "source")
		)` + t.suffix,

		`CREATE INDEX "role_grant_person_idx" ON "role_grant" ("person_id")`,

		// The ways one person may sign in.
		//
		// Two things go wrong when a username is the whole identity. A
		// username moves — people change their name at work, and a forge login
		// can be renamed and the old one then claimed by somebody else — so
		// matching on it eventually hands one person's access to another. And
		// a username is only unique within the provider that issued it, so a
		// deployment with two providers configured would treat the same name
		// from each as one person.
		//
		// So a sign-in is matched on the provider's own stable identifier, and
		// the username is what an administrator types to authorize somebody
		// before that identifier is knowable.
		//
		// subject is absent until the first successful sign-in binds it. All
		// four engines treat NULLs in a unique key as distinct from each
		// other, so many rows may await binding while no two bound rows share
		// a subject — checked on each engine rather than assumed, because the
		// opposite behavior is a configurable option on one of them.
		`CREATE TABLE "person_identity" (
			"id"         ` + t.id + `,
			"person_id"  ` + t.ref + ` NOT NULL,
			"provider"   ` + t.name + ` NOT NULL,
			"subject"    ` + t.name + ` NULL,
			"username"   ` + t.name + ` NOT NULL,
			"created_at" ` + t.timestamp + ` NOT NULL,
			"bound_at"   ` + t.timestamp + ` NULL,
			CONSTRAINT "person_identity_person_fk" FOREIGN KEY ("person_id") REFERENCES "person"("id"),
			CONSTRAINT "person_identity_username_unique" UNIQUE ("provider", "username"),
			CONSTRAINT "person_identity_subject_unique" UNIQUE ("provider", "subject")
		)` + t.suffix,

		`CREATE INDEX "person_identity_person_idx" ON "person_identity" ("person_id")`,

		// A pipeline's credential. Ingest and nothing else: a build server has
		// no business holding a person's permissions, which is also what keeps
		// the visibility rules out of its reach entirely (ACC-10).
		//
		// The secret is stored hashed and shown once (SEC-03). A store
		// readable by whoever reads the database is a store that hands over
		// every pipeline's credential with it.
		//
		// The scope is a set of constraints rather than a path: the product is
		// always required, and the release and the variant are independent and
		// either, both or neither may be pinned. An upload always states its
		// full target; the key only authorizes it (ACC-11).
		`CREATE TABLE "api_key" (
			"id"           ` + t.id + `,
			"name"         ` + t.name + ` NOT NULL,
			"secret_hash"  ` + t.hash + ` NOT NULL,
			"product_id"   ` + t.ref + ` NOT NULL,
			"stream_id"    ` + t.refNull + ` NULL,
			"variant_id"   ` + t.refNull + ` NULL,
			"created_at"   ` + t.timestamp + ` NOT NULL,
			"last_used_at" ` + t.timestamp + ` NULL,
			"revoked_at"   ` + t.timestamp + ` NULL,
			CONSTRAINT "api_key_product_fk" FOREIGN KEY ("product_id") REFERENCES "product"("id"),
			CONSTRAINT "api_key_stream_fk" FOREIGN KEY ("stream_id") REFERENCES "stream"("id"),
			CONSTRAINT "api_key_variant_fk" FOREIGN KEY ("variant_id") REFERENCES "variant"("id"),
			CONSTRAINT "api_key_secret_unique" UNIQUE ("secret_hash"),
			-- The name is what an ingest records as having sent a scan, and
			-- what an administrator revokes by. Two keys sharing one would
			-- make each able to read the other's receipts, and would make a
			-- revocation report success having withdrawn whichever row came
			-- back first.
			CONSTRAINT "api_key_name_unique" UNIQUE ("name")
		)` + t.suffix,

		`CREATE INDEX "api_key_product_idx" ON "api_key" ("product_id")`,

		// A person's session, held here rather than in a process's memory:
		// several copies of the application may answer, and a session has to
		// work whichever one does. Storing it also makes revocation immediate
		// — deleting the row cuts access off at once (ACC-16), which is the
		// mechanism relied on when somebody leaves, because group membership
		// is only ever re-read at the next sign-in (ACC-38).
		//
		// The token is stored hashed for the same reason a key is: a store
		// that can hand back what it holds hands over every live session along
		// with a copy of the database. It is not derived from anything about
		// the person, so there is nothing to guess at.
		//
		// csrf_token is a second value bound to the same session. The session
		// cookie is sent by the browser automatically, which is what makes a
		// hostile page able to act as the signed-in user; this one has to be
		// read and echoed by script that the same-origin policy allows only
		// our own pages to run (ACC-18).
		`CREATE TABLE "session" (
			"id"           ` + t.id + `,
			"token_hash"   ` + t.hash + ` NOT NULL,
			"csrf_token"   ` + t.hash + ` NOT NULL,
			"person_id"    ` + t.ref + ` NOT NULL,
			"created_at"   ` + t.timestamp + ` NOT NULL,
			"expires_at"   ` + t.timestamp + ` NOT NULL,
			"last_used_at" ` + t.timestamp + ` NULL,
			CONSTRAINT "session_person_fk" FOREIGN KEY ("person_id") REFERENCES "person"("id"),
			CONSTRAINT "session_token_unique" UNIQUE ("token_hash")
		)` + t.suffix,

		`CREATE INDEX "session_person_idx" ON "session" ("person_id")`,
		// Expired rows are cleared in bulk rather than one at a time.
		`CREATE INDEX "session_expires_idx" ON "session" ("expires_at")`,

		// A provider group bound to a role on a product. In group-bound mode
		// this table *is* the pre-authorization: somebody arriving for the
		// first time in a mapped group is admitted and recorded then, which is
		// what reconciles admitting them with never creating an account for
		// somebody nobody authorized (ACC-27).
		//
		// The group is matched by the name the provider reports — a team slug
		// from GitHub, a claim value from an identity provider — so it is
		// stored as given rather than resolved to anything of ours.
		`CREATE TABLE "group_role" (
			"id"         ` + t.id + `,
			"group_name" ` + t.name + ` NOT NULL,
			"product_id" ` + t.ref + ` NOT NULL,
			"role"       ` + t.kind + ` NOT NULL,
			"created_at" ` + t.timestamp + ` NOT NULL,
			CONSTRAINT "group_role_product_fk" FOREIGN KEY ("product_id") REFERENCES "product"("id"),
			CONSTRAINT "group_role_unique" UNIQUE ("group_name", "product_id", "role")
		)` + t.suffix,

		`CREATE INDEX "group_role_name_idx" ON "group_role" ("group_name")`,

		// Somebody's own credential for scripting. It is a live reference to
		// its owner rather than a snapshot of what they could do when it was
		// minted: what it reaches shrinks the moment their roles shrink, and
		// it dies with their account. A snapshot would quietly outlive the
		// access it was granted from — including a role withdrawn by a group
		// membership going away, which is the case with nothing to notice it
		// (ACC-34).
		//
		// expires_at is not nullable. A credential that never expires is one
		// nobody ever revokes, and the maximum is an administrator's to set
		// (ACC-33).
		`CREATE TABLE "personal_token" (
			"id"           ` + t.id + `,
			"name"         ` + t.name + ` NOT NULL,
			"secret_hash"  ` + t.hash + ` NOT NULL,
			"person_id"    ` + t.ref + ` NOT NULL,
			-- product_id narrows a token below its owner rather than above:
			-- what it reaches is the intersection, so pinning it to something
			-- they cannot read reaches nothing rather than granting it.
			"product_id"   ` + t.refNull + ` NULL,
			"created_at"   ` + t.timestamp + ` NOT NULL,
			"expires_at"   ` + t.timestamp + ` NOT NULL,
			"last_used_at" ` + t.timestamp + ` NULL,
			"revoked_at"   ` + t.timestamp + ` NULL,
			CONSTRAINT "personal_token_person_fk" FOREIGN KEY ("person_id") REFERENCES "person"("id"),
			CONSTRAINT "personal_token_product_fk" FOREIGN KEY ("product_id") REFERENCES "product"("id"),
			CONSTRAINT "personal_token_secret_unique" UNIQUE ("secret_hash"),
			-- Its owner names it and withdraws it by that name, so two of
			-- theirs may not share one.
			CONSTRAINT "personal_token_name_unique" UNIQUE ("person_id", "name")
		)` + t.suffix,

		`CREATE INDEX "personal_token_person_idx" ON "personal_token" ("person_id")`,

		// Admin is global rather than held against a product (ACC-07), so a
		// group mapping to it cannot live in the table above: the product
		// would have to be absent, and a uniqueness rule over a column that
		// may be absent behaves differently on each of the four engines. A
		// second table with one column costs less than that difference.
		//
		// At least one row here is required while group-bound mode is on,
		// checked at startup, or a deployment can lock itself out of its own
		// administration (ACC-28).
		`CREATE TABLE "group_admin" (
			"id"         ` + t.id + `,
			"group_name" ` + t.name + ` NOT NULL,
			"created_at" ` + t.timestamp + ` NOT NULL,
			CONSTRAINT "group_admin_unique" UNIQUE ("group_name")
		)` + t.suffix,
	}

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func downAccess(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		`DROP TABLE "personal_token"`,
		`DROP TABLE "person_identity"`,
		`DROP TABLE "group_admin"`,
		`DROP TABLE "group_role"`,
		`DROP TABLE "session"`,
		`DROP TABLE "api_key"`,
		`DROP TABLE "role_grant"`,
		`DROP TABLE "person"`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
