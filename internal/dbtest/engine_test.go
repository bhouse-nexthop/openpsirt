package dbtest_test

import (
	"strings"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
)

// The engines gate greps this test's output for each engine by name, and until
// now that only proved a subtest with that *label* had run. The label comes
// from a list in this package; the connection comes from an environment
// variable. Nothing checked that the two agreed.
//
// So pointing the MySQL and MariaDB URLs at the same PostgreSQL server — four
// URLs differing only in a port digit, which is exactly the slip somebody
// makes — produced four green lines and "every engine ran". The suite would
// then have tested one engine three times while reporting three.
//
// This asks the server what it is and compares that against the name the
// harness gave it.
func TestEachEngineIsTheEngineItSaysItIs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		// The **label**, not the engine the connection thinks it is. Those are
		// two different facts and comparing the connection against itself
		// proves nothing: a MySQL URL pointed at a PostgreSQL server opens as
		// PostgreSQL and agrees with itself perfectly, while the gate greps
		// for a line saying "mysql". The label is what the gate believes, so
		// the label is what has to be checked.
		parts := strings.Split(t.Name(), "/")
		labelled := database.Engine(parts[len(parts)-1])

		// The URL's own scheme must match the label. This is what catches the
		// likeliest slip — four URLs differing by a port digit, one pasted
		// over another — because a `postgres://` URL under the `mysql` label
		// opens as PostgreSQL and then agrees with itself about being
		// PostgreSQL.
		if db.Server.Engine != labelled {
			t.Fatalf("the connection labelled %s opened as %s, so this run "+
				"tested a different engine than it reported", labelled, db.Server.Engine)
		}

		var said string
		query := "SELECT version()"
		if labelled == database.SQLite {
			query = "SELECT sqlite_version()"
		}
		if err := db.QueryRowContext(t.Context(), query).Scan(&said); err != nil {
			t.Fatalf("ask the server what it is: %v", err)
		}

		// What each engine's own version string contains. MariaDB reports
		// itself through MySQL's function and says "MariaDB" in the text,
		// which is the only thing telling the two apart from here.
		want := map[database.Engine]string{
			database.Postgres: "postgresql",
			database.MariaDB:  "mariadb",
		}[labelled]

		lower := strings.ToLower(said)
		switch labelled {
		case database.SQLite:
			// sqlite_version() answers on SQLite and nowhere else, so getting
			// an answer at all is the check. And the connection has to agree
			// it is SQLite, or the URL points somewhere else entirely.
			if db.Server.Engine != database.SQLite {
				t.Errorf("the connection labelled sqlite opened as %s", db.Server.Engine)
			}
		case database.MySQL:
			// MySQL is the awkward one: it has no banner of its own, so it is
			// named by what it must *not* say. MariaDB answers the same
			// function, and a wrong URL could reach anything.
			for _, other := range []string{"mariadb", "postgresql", "sqlite"} {
				if strings.Contains(lower, other) {
					t.Errorf("the connection labelled mysql reports itself as %q", said)
				}
			}
		default:
			if !strings.Contains(lower, want) {
				t.Errorf("the connection labelled %s reports itself as %q, "+
					"so this run tested a different engine than it said",
					labelled, said)
			}
		}
	})
}
