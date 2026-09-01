package currency_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/currency"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
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
	// version is what a previous pass stored. Its presence is what tells a
	// stale answer apart from a question the index could not answer, which
	// are left alone for very different lengths of time.
	version *string
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
			LatestVersion:   each.version,
		}
		if _, insert := db.DB.NewInsert().Model(row).Exec(ctx); insert != nil {
			t.Fatalf("seed component %d: %v", i, insert)
		}
	}
	// Asking is off unless a deployment turns it on, and `Once` enforces that
	// on every component rather than trusting `Run` to have checked — a guard
	// beside the work cannot be skipped by calling the work another way.
	if err := setting.NewStore(db.DB).Set(ctx, setting.UpstreamCurrency, setting.On); err != nil {
		t.Fatalf("turn asking on: %v", err)
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
func TestAPackageNobodyPublishesIsLeftAloneForAMonth(t *testing.T) {
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

		// And it is left alone for a month rather than asked again daily,
		// which is the point: this question has no answer and asking it every
		// day is thousands of pointless requests at somebody else's free
		// service.
		for _, when := range []struct {
			what  string
			later time.Duration
			due   int
		}{
			{"a day later", currency.StaleAfter + time.Hour, 0},
			{"a week later", 7 * 24 * time.Hour, 0},
			{"a month later", currency.UnknownAfter + time.Hour, 1},
		} {
			second, _ := seed(t, db, nil, nil, nil)
			at := time.Now().Add(when.later)
			second.Now = func() time.Time { return at }
			asked, err := second.Once(t.Context())
			if err != nil {
				t.Fatalf("%s: %v", when.what, err)
			}
			if asked != when.due {
				t.Errorf("%s: asked about %d, expected %d", when.what, asked, when.due)
			}
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
		had := "1.0.0"
		r, asked := seed(t, db, []component{
			{purl: "pkg:cargo/fresh@1.0.0", checked: &recent, version: &had},
			{purl: "pkg:cargo/stale@1.0.0", checked: &old, version: &had},
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

// The message this drives tells somebody that waiting for a fix is unlikely to
// work, so the evidence for it has to be a real silence rather than an
// artefact of comparing two year-numbers.
func TestUpstreamIsOnlyCalledSilentAfterAClearYear(t *testing.T) {
	for _, c := range []struct {
		what       string
		identifier string
		released   time.Time
		silent     bool
	}{
		{"released weeks before it was named",
			"CVE-2026-31431", time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC), false},
		{"released the same year",
			"CVE-2026-31431", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), false},
		{"released the whole year before",
			"CVE-2026-31431", time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), false},
		{"silent for a clear year",
			"CVE-2026-31431", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), true},
		{"silent for years",
			"CVE-2021-44228", time.Date(2017, 6, 1, 0, 0, 0, 0, time.UTC), true},
		// An identifier that names no year says nothing either way, and
		// guessing would invent the fact this exists to supply.
		{"no year in the identifier",
			"GHSA-jfh8-c2jp-5v3q", time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), false},
		{"nothing known about upstream", "CVE-2026-31431", time.Time{}, false},
	} {
		if got := currency.NothingSince(c.identifier, c.released); got != c.silent {
			t.Errorf("%s: NothingSince(%q, %s) = %v, expected %v",
				c.what, c.identifier, c.released.Format(time.DateOnly), got, c.silent)
		}
	}
}

// The defect this exists to prevent: a component whose ecosystem has no index
// used to be skipped without recording that we had looked, so it stayed due
// forever. `due` takes the oldest 200 with never-asked first, so on a real
// image — 3,929 components in ecosystems nothing asks against 3,010 that are
// askable — the window filled with rows nothing ever wrote and the feature
// asked upstream about nothing at all, every cycle, for good.
func TestEcosystemsWithNoIndexDoNotStarveTheQueue(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		var of []component
		for i := 0; i < currency.MostPerPass; i++ {
			of = append(of, component{purl: fmt.Sprintf("pkg:generic/thing-%03d@1.0", i)})
		}
		of = append(of, component{purl: "pkg:cargo/serde@1.0.0"})
		r, asked := seed(t, db, of, map[string]currency.Latest{
			"serde": {Version: "1.0.230"},
		}, nil)

		// However the unaskable ones are ordered, they must be got through
		// rather than sat on. Two passes is more than enough for one slice.
		for range 2 {
			if _, err := r.Once(t.Context()); err != nil {
				t.Fatalf("once: %v", err)
			}
		}
		if len(*asked) == 0 {
			t.Fatal("nothing upstream was ever asked: the unaskable components " +
				"filled the window and were never recorded, so they stayed due")
		}
		unasked := 0
		for _, row := range read(t, db) {
			if row.Checked == nil {
				unasked++
			}
		}
		if unasked != 0 {
			t.Errorf("%d components still have no record of being looked at", unasked)
		}
	})
}

// A name that cannot be made into a request will never come right, so it is
// recorded rather than retried. Left retryable it starves the queue the same
// way, and one uploaded document full of such names stops the worker.
func TestANameThatCannotBeAskedAboutIsNotRetriedForever(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		r, _ := seed(t, db, []component{{purl: "pkg:golang/example.com/m@v1"}},
			nil, currency.ErrUnaskable)

		if _, err := r.Once(t.Context()); err != nil {
			t.Fatalf("once: %v", err)
		}
		got := read(t, db)["pkg:golang/example.com/m@v1"]
		if got.Checked == nil {
			t.Error("a name that cannot be asked about was left due forever, " +
				"so it will fill the window on every pass from now on")
		}
	})
}

// Turning it off has to take effect without a redeploy, which is the reason
// the setting is read per cycle rather than at startup. A pass is up to 200
// requests with a timeout each, so reading it only at the top of a pass leaves
// an operator who has just turned it off waiting the better part of an hour.
func TestTurningItOffStopsTheCurrentPass(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		var of []component
		for i := 0; i < 10; i++ {
			of = append(of, component{purl: fmt.Sprintf("pkg:cargo/crate-%d@1.0", i)})
		}
		r, asked := seed(t, db, of, map[string]currency.Latest{}, nil)

		// Off again after the first component is asked about.
		settings := setting.NewStore(db.DB)
		r.Index = func(string) currency.Asker {
			return stopping{t: t, settings: settings, asked: asked}
		}
		if _, err := r.Once(t.Context()); err != nil {
			t.Fatalf("once: %v", err)
		}
		if len(*asked) > 2 {
			t.Errorf("asked about %d components after it was turned off: %v",
				len(*asked), *asked)
		}
	})
}

// stopping answers once and turns the setting off while the pass is running.
type stopping struct {
	t        *testing.T
	settings *setting.Store
	asked    *[]string
}

func (s stopping) Latest(ctx context.Context, name string) (currency.Latest, error) {
	*s.asked = append(*s.asked, name)
	if err := s.settings.Set(ctx, setting.UpstreamCurrency, setting.Off); err != nil {
		s.t.Fatalf("turn asking off: %v", err)
	}
	return currency.Latest{Version: "1.0.0"}, nil
}

// An answer we already had is not thrown away because the index said 404 once.
// npm and crates.io return one for a renamed package and for some transient
// conditions, and overwriting would destroy a version we had and then sit on
// the hole for a month.
func TestAnIndexSayingNoDoesNotDestroyWhatWeAlreadyKnew(t *testing.T) {
	each(t, func(t *testing.T, db *database.DB) {
		had := "1.0.0"
		long := time.Now().UTC().Add(-2 * currency.StaleAfter)
		r, _ := seed(t, db, []component{
			{purl: "pkg:cargo/serde@1.0.0", checked: &long, version: &had},
		}, map[string]currency.Latest{}, nil)

		if _, err := r.Once(t.Context()); err != nil {
			t.Fatalf("once: %v", err)
		}
		got := read(t, db)["pkg:cargo/serde@1.0.0"]
		if got.Version == nil || *got.Version != had {
			t.Errorf("the stored version became %v; one 404 must not destroy it", got.Version)
		}
		if got.Checked == nil || !got.Checked.After(long) {
			t.Error("the time of asking was not moved on")
		}
	})
}
