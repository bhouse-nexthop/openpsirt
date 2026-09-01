package currency_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/currency"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// answers is an index that says what a test tells it to, so none of this
// reaches the network. The real ones are somebody else's service and their
// answers change, which is not something to assert against.
type answers struct {
	says map[string]currency.Latest
	err  error
	// asked records every name this was asked about, in order, so a test can
	// say what was *not* asked as well as what was.
	asked *[]string
}

func (a answers) Latest(_ context.Context, name string) (currency.Latest, error) {
	*a.asked = append(*a.asked, name)
	if a.err != nil {
		return currency.Latest{}, a.err
	}
	latest, known := a.says[name]
	if !known {
		return currency.Latest{}, currency.ErrUnknown
	}
	return latest, nil
}

type component struct {
	purl    string
	checked *time.Time
}

// seed puts components in the graph and returns a refresher over them.
func seed(t *testing.T, db *database.DB, of []component,
	says map[string]currency.Latest, err error) (*currency.Refresher, *[]string) {

	t.Helper()
	ctx := t.Context()
	for i, each := range of {
		row := &graph.Component{
			Identity:        "identity-" + each.purl,
			Purl:            each.purl,
			Name:            "component",
			Version:         "1.0",
			FirstSeenAt:     time.Now().UTC(),
			LatestCheckedAt: each.checked,
		}
		if _, insert := db.DB.NewInsert().Model(row).Exec(ctx); insert != nil {
			t.Fatalf("seed component %d: %v", i, insert)
		}
	}
	asked := &[]string{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := currency.NewRefresher(db.DB, quiet)
	r.Pause = 0
	r.Index = func(ecosystem string) currency.Asker {
		switch ecosystem {
		case "golang", "npm", "pypi", "cargo":
			return answers{says: says, err: err, asked: asked}
		}
		return nil
	}
	return r, asked
}

type stored struct {
	Purl     string     `bun:"purl"`
	Version  *string    `bun:"latest_version"`
	Released *time.Time `bun:"latest_released_at"`
	Checked  *time.Time `bun:"latest_checked_at"`
}

func read(t *testing.T, db *database.DB) map[string]stored {
	t.Helper()
	var rows []stored
	err := db.DB.NewSelect().
		TableExpr("component AS c").
		ColumnExpr("c.purl, c.latest_version, c.latest_released_at, c.latest_checked_at").
		Scan(t.Context(), &rows)
	if err != nil {
		t.Fatalf("read components: %v", err)
	}
	out := map[string]stored{}
	for _, row := range rows {
		out[row.Purl] = row
	}
	return out
}

func each(t *testing.T, fn func(t *testing.T, db *database.DB)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(t.Context(), db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)
		fn(t, db)
	})
}

func TestWhatUpstreamHasReleasedIsRecorded(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		shipped := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		r, _ := seed(t, db, []component{
			{purl: "pkg:golang/golang.org/x/net@v0.17.0"},
		}, map[string]currency.Latest{
			"golang.org/x/net": {Version: "v0.38.0", Released: shipped},
		}, nil)

		asked, err := r.Once(t.Context())
		if err != nil {
			t.Fatalf("once: %v", err)
		}
		if asked != 1 {
			t.Fatalf("asked about %d components, expected 1", asked)
		}
		got := read(t, db)["pkg:golang/golang.org/x/net@v0.17.0"]
		if got.Version == nil || *got.Version != "v0.38.0" {
			t.Errorf("newest version is %v, expected v0.38.0", got.Version)
		}
		if got.Released == nil || !got.Released.UTC().Equal(shipped) {
			t.Errorf("released at %v, expected %v", got.Released, shipped)
		}
		if got.Checked == nil {
			t.Error("nothing recorded that we asked")
		}
	})
}

// A distribution package has a maintainer, and it is the distribution. Asking
// an upstream index when its newest release shipped says nothing about the age
// of what Debian is carrying, and answering as though it did would be worse
// than not answering.
func TestADistributionPackageIsNotAskedAbout(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		r, asked := seed(t, db, []component{
			{purl: "pkg:deb/debian/openssl@3.5.6-1"},
			{purl: ""},
			{purl: "pkg:cargo/serde@1.0.0"},
		}, map[string]currency.Latest{"serde": {Version: "1.0.230"}}, nil)

		if _, err := r.Once(t.Context()); err != nil {
			t.Fatalf("once: %v", err)
		}
		if len(*asked) != 1 || (*asked)[0] != "serde" {
			t.Fatalf("asked about %v, expected only serde", *asked)
		}
		stored := read(t, db)
		if stored["pkg:deb/debian/openssl@3.5.6-1"].Checked != nil {
			t.Error("a distribution package was recorded as asked about")
		}
	})
}

// A private module and a vendored fork both look like a package the index has
// never heard of. Recording that we asked is what stops it being asked again
// tomorrow, and every day after, forever.
func TestAPackageNobodyPublishesIsNotAskedAboutForever(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		r, _ := seed(t, db, []component{
			{purl: "pkg:golang/github.com/example/private@v1.0.0"},
		}, map[string]currency.Latest{}, nil)

		if _, err := r.Once(t.Context()); err != nil {
			t.Fatalf("once: %v", err)
		}
		got := read(t, db)["pkg:golang/github.com/example/private@v1.0.0"]
		if got.Checked == nil {
			t.Fatal("an unanswerable question was not recorded as asked")
		}
		if got.Version != nil {
			t.Errorf("a version was stored for a package nobody publishes: %v", *got.Version)
		}

		// And it is not due again, which is the point.
		second, _ := seed(t, db, nil, nil, nil)
		asked, err := second.Once(t.Context())
		if err != nil {
			t.Fatalf("second pass: %v", err)
		}
		if asked != 0 {
			t.Errorf("asked again immediately about %d components", asked)
		}
	})
}

// An index having a bad day must not be recorded as an answer, or a failure
// becomes a stored fact that nothing revisits for a day.
func TestAnIndexThatFailsIsAskedAgain(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		r, _ := seed(t, db, []component{
			{purl: "pkg:pypi/django@4.2"},
		}, nil, context.DeadlineExceeded)

		asked, err := r.Once(t.Context())
		if err != nil {
			t.Fatalf("once: %v", err)
		}
		if asked != 0 {
			t.Errorf("counted %d as asked when the index failed", asked)
		}
		if read(t, db)["pkg:pypi/django@4.2"].Checked != nil {
			t.Error("a failed request was recorded as having been asked")
		}
	})
}

// Asked recently is left alone; asked long enough ago is asked again.
func TestOnlyWhatIsStaleIsAskedAgain(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		recent := time.Now().UTC().Add(-time.Hour)
		old := time.Now().UTC().Add(-2 * currency.StaleAfter)
		r, asked := seed(t, db, []component{
			{purl: "pkg:cargo/fresh@1.0.0", checked: &recent},
			{purl: "pkg:cargo/stale@1.0.0", checked: &old},
		}, map[string]currency.Latest{
			"fresh": {Version: "9"}, "stale": {Version: "9"},
		}, nil)

		if _, err := r.Once(t.Context()); err != nil {
			t.Fatalf("once: %v", err)
		}
		if len(*asked) != 1 || (*asked)[0] != "stale" {
			t.Fatalf("asked about %v, expected only the stale one", *asked)
		}
	})
}

func TestTheNameAskedComesFromTheIdentifier(t *testing.T) {
	for _, c := range []struct {
		purl      string
		ecosystem string
		name      string
		ok        bool
	}{
		{"pkg:golang/golang.org/x/net@v0.17.0", "golang", "golang.org/x/net", true},
		{"pkg:npm/%40types/node@20.1.0", "npm", "@types/node", true},
		{"pkg:pypi/django@4.2", "pypi", "django", true},
		{"pkg:cargo/serde@1.0.0?arch=amd64", "cargo", "serde", true},
		// The subpath follows the qualifiers and is neither of them.
		{"pkg:golang/example.com/m@v1#sub/dir", "golang", "example.com/m", true},
		{"", "", "", false},
		{"not-a-purl", "", "", false},
		{"pkg:golang", "", "", false},
	} {
		ecosystem, name, ok := currency.Asked(c.purl)
		if ok != c.ok || ecosystem != c.ecosystem || name != c.name {
			t.Errorf("Asked(%q) = %q, %q, %v; expected %q, %q, %v",
				c.purl, ecosystem, name, ok, c.ecosystem, c.name, c.ok)
		}
	}
}
