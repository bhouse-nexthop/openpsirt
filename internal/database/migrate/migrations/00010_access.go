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
// because it is the one role that is global (ACC-07). Modelling it as a grant
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
		`CREATE TABLE person (
			id           ` + t.id + `,
			identity     ` + t.name + ` NOT NULL,
			display_name ` + t.free + ` NULL,
			is_admin     ` + t.boolean + ` NOT NULL,
			created_at   ` + t.timestamp + ` NOT NULL,
			last_seen_at ` + t.timestamp + ` NULL,
			CONSTRAINT person_identity_unique UNIQUE (identity)
		)` + t.suffix,

		// A role is held against one product. What somebody may reach is the
		// union of their grants, and a product they hold no grant on is not
		// merely unreadable but invisible (ACC-08).
		`CREATE TABLE role_grant (
			id         ` + t.id + `,
			person_id  ` + t.ref + ` NOT NULL REFERENCES person(id),
			product_id ` + t.ref + ` NOT NULL REFERENCES product(id),
			role       ` + t.kind + ` NOT NULL,
			created_at ` + t.timestamp + ` NOT NULL,
			CONSTRAINT role_grant_unique UNIQUE (person_id, product_id, role)
		)` + t.suffix,

		`CREATE INDEX role_grant_person_idx ON role_grant (person_id)`,

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
		`CREATE TABLE api_key (
			id           ` + t.id + `,
			name         ` + t.name + ` NOT NULL,
			secret_hash  ` + t.hash + ` NOT NULL,
			product_id   ` + t.ref + ` NOT NULL REFERENCES product(id),
			stream_id    ` + t.refNull + ` NULL REFERENCES stream(id),
			variant_id   ` + t.refNull + ` NULL REFERENCES variant(id),
			created_at   ` + t.timestamp + ` NOT NULL,
			last_used_at ` + t.timestamp + ` NULL,
			revoked_at   ` + t.timestamp + ` NULL,
			CONSTRAINT api_key_secret_unique UNIQUE (secret_hash)
		)` + t.suffix,

		`CREATE INDEX api_key_product_idx ON api_key (product_id)`,
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
		`DROP TABLE api_key`,
		`DROP TABLE role_grant`,
		`DROP TABLE person`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
