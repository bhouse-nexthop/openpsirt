package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// betweenSweeps is how often the conditions are re-derived.
//
// They are about a threshold measured in days, so asking every few minutes
// would be answering a question nobody asked more often than the answer can
// change. Long enough to be cheap, short enough that somebody who fixes a
// pipeline sees the alert go while they are still looking at it.
const betweenSweeps = 15 * time.Minute

// Watch derives the conditions that are true and tells the people who should
// know.
//
// Operational alerts are their own category (NTF-07): they are about the
// tool's own health rather than about anybody's work, they are not an explicit
// human action so they are never immediate mail, and a digest that is off by
// default would carry them to nobody. They go to administrators, who are the
// people who can act on them.
//
// Everything here is a condition rather than an event (NTF-09). A build that
// stopped being scanned is true until it is scanned again, and then it is not
// — which is a thing the pass discovers rather than a thing anybody dismisses.
type Watch struct {
	db     *bun.DB
	logger *slog.Logger
}

// NewWatch returns a watch over db.
func NewWatch(db *bun.DB, logger *slog.Logger) *Watch {
	return &Watch{db: db, logger: logger}
}

// Run sweeps until the context ends.
func (w *Watch) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = betweenSweeps
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if opened, cleared, err := w.Once(ctx); err != nil {
			// Logged and carried on, like every other background pass here: a
			// sweep that cannot run is not a reason to stop the process, and
			// the next one is fifteen minutes away.
			w.logger.Error("working out what is worth saying", "error", err)
		} else if opened > 0 || cleared > 0 {
			w.logger.Info("operational alerts changed",
				"opened", opened, "cleared", cleared)
		}
		if gone, err := w.Tidy(ctx); err != nil {
			w.logger.Error("clearing expired sessions", "error", err)
		} else if gone > 0 {
			w.logger.Info("cleared expired sessions", "sessions", gone)
		}
		timer.Reset(interval)
	}
}

// Once derives every condition and reconciles it against what is being said.
func (w *Watch) Once(ctx context.Context) (opened, cleared int, err error) {
	admins, err := w.administrators(ctx)
	if err != nil {
		return 0, 0, err
	}
	if len(admins) == 0 {
		// Nothing to say and nobody to say it to. Not an error: a deployment
		// with no administrator recorded cannot start, so this is the window
		// between the table existing and the first sign-in.
		return 0, 0, nil
	}

	quiet, err := w.quietBuilds(ctx)
	if err != nil {
		return 0, 0, err
	}

	// The same conditions to each of them. An alert about the tool's health is
	// not somebody's personal work item, and the first administrator to look
	// should not be the only one who ever sees it.
	for _, admin := range admins {
		o, c, err := NewStore(w.db).Reconcile(ctx, admin, BuildQuiet, quiet)
		if err != nil {
			return opened, cleared, fmt.Errorf("tell %d what has gone quiet: %w", admin, err)
		}
		opened += o
		cleared += c
	}

	// Somebody away and still holding work (ACC-45). The same list to every
	// administrator, for the same reason the quiet builds are: whichever of
	// them looks first should not be the only one who ever sees it.
	away, err := w.holdingAbsent(ctx)
	if err != nil {
		return opened, cleared, err
	}
	for _, admin := range admins {
		o, c, err := NewStore(w.db).Reconcile(ctx, admin, HoldingAbsent, away)
		if err != nil {
			return opened, cleared, fmt.Errorf("tell %d who is away: %w", admin, err)
		}
		opened += o
		cleared += c
	}

	// An embargo whose date has arrived is not a fact about the tool's health,
	// so it is not the same list to the same people: administrators hear about
	// all of them, and whoever holds one hears about theirs. Reconcile makes
	// somebody's open set exactly what it is handed, so each person's whole
	// list is worked out before any of it is written — an administrator who is
	// also holding one has to be handed both halves at once or the second call
	// would clear the first.
	due, err := w.pastDisclosure(ctx, admins)
	if err != nil {
		return opened, cleared, err
	}
	for person, holding := range due {
		o, c, err := NewStore(w.db).Reconcile(ctx, person, DisclosureDue, holding)
		if err != nil {
			return opened, cleared, fmt.Errorf("tell %d what is past its date: %w", person, err)
		}
		opened += o
		cleared += c
	}

	// A critical against something already shipped (NTF-11). Like the embargo
	// list and unlike the two above it, this is per person rather than the
	// same list to everybody: it names an issue at a build, which is finding
	// content, so who hears it follows who may read it.
	shipped, err := w.criticalOnReleases(ctx)
	if err != nil {
		return opened, cleared, err
	}
	for person, holding := range shipped {
		o, c, err := NewStore(w.db).Reconcile(ctx, person, CriticalOnRelease, holding)
		if err != nil {
			return opened, cleared,
				fmt.Errorf("tell %d what is critical on a release: %w", person, err)
		}
		opened += o
		cleared += c
	}
	return opened, cleared, nil
}

// criticalOnReleases is every critical or actively-exploited issue open and
// undecided against a tag, against the people who should hear about it.
//
// **Tags only.** A critical on a branch is ordinary work in progress; the same
// issue against something customers are running is the case NTF-11 exists for,
// and the difference between them is the whole signal. Sending both would make
// the alert as common as the findings list and therefore ignorable.
//
// **Undecided only**, and derived rather than remembered, so it leaves the
// list when the finding closes or when somebody answers it — neither of which
// is a thing to be dismissed. That is the NTF-09 shape: nobody clears this,
// the world does.
//
// **Whoever may read it and may act on it** (NTF-19). Every other operational
// alert goes to administrators because it is about the tool rather than about
// a finding; this one names an issue, a component and a build, which is
// finding content. Since an administrator no longer reads a product by
// administering it (ACC-64), sending it to administrators alone would be both
// a disclosure to people who may not read it and silence for the people who
// can act.
func (w *Watch) criticalOnReleases(ctx context.Context) (map[int64][]Holds, error) {
	standing, args := finding.OffTheClock("st.product_id", time.Now().UTC())

	var rows []struct {
		Product       string `bun:"product"`
		Stream        string `bun:"stream"`
		Variant       string `bun:"variant"`
		Component     string `bun:"component"`
		Vulnerability string `bun:"vulnerability"`
		ProductID     int64  `bun:"product_id"`
		Visibility    string `bun:"visibility"`
		Exploited     bool   `bun:"exploited"`
	}
	err := w.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN product AS p ON p.id = st.product_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		// The consumer, because whether a decision still covers a place is
		// keyed on both upstream versions and one of them is the consumer's.
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("p.name AS product").
		ColumnExpr("st.name AS stream").
		ColumnExpr("va.name AS variant").
		ColumnExpr("c.name AS component").
		ColumnExpr("v.identifier AS vulnerability").
		ColumnExpr("st.product_id AS product_id").
		ColumnExpr("MIN(f.visibility) AS visibility").
		ColumnExpr("MAX(CASE WHEN f.urgency_exploited THEN 1 ELSE 0 END) = 1 AS exploited").
		Where("st.kind = ?", catalog.Tag).
		Where("f.closed_at IS NULL").
		Where("f.suppressed_by IS NULL").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereOr("f.urgency_exploited = ?", true).
				WhereOr(finding.BandExpr+" = ?", "critical")
		}).
		Where("NOT "+standing, args...).
		GroupExpr("p.name, st.name, va.name, c.name, v.identifier, st.product_id").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read what is critical on a release: %w", err)
	}

	// Who may read each product, at each visibility, and who may act there.
	// Read once rather than per row: a release with a thousand criticals would
	// otherwise ask the same question a thousand times.
	people, held, err := access.NewStore(w.db).People(ctx)
	if err != nil {
		return nil, fmt.Errorf("read who may hear about this: %w", err)
	}
	type reach struct{ public, private bool }
	acts := map[int64]map[int64]reach{}
	for _, person := range people {
		per := map[int64]reach{}
		for _, grant := range held[person.ID] {
			if !grant.Active {
				continue
			}
			at := per[grant.ProductID]
			switch grant.Role {
			case access.PublicTriage:
				at.public = true
			case access.PrivateTriage:
				at.public, at.private = true, true
			}
			per[grant.ProductID] = at
		}
		acts[person.ID] = per
	}

	// Everybody currently being told one of these is handed a list, empty
	// included. Reconcile makes one person's open set exactly what it is
	// given, so somebody who is never handed a list is never reconciled, and
	// their alert would stand after the thing it was about had been answered.
	out := map[int64][]Holds{}
	told, err := w.beingTold(ctx, CriticalOnRelease)
	if err != nil {
		return nil, err
	}
	for _, person := range told {
		out[person] = nil
	}
	for personID := range acts {
		if _, already := out[personID]; !already {
			out[personID] = nil
		}
	}

	for _, row := range rows {
		private := row.Visibility == string(access.Private)
		where := row.Product + " " + row.Stream + " " + row.Variant
		why := "is rated critical"
		if row.Exploited {
			why = "is being exploited"
		}
		holds := Holds{
			About: identify("critical-on-release " + where + " " + row.Vulnerability + " " + row.Component),
			Body: fmt.Sprintf("%s %s and is open against %s, which has been released. Nothing has been decided about it.",
				row.Vulnerability, why, where),
			Link: fmt.Sprintf("/products/%s/streams/%s/variants/%s/findings/%s/components/%s",
				url.PathEscape(row.Product), url.PathEscape(row.Stream), url.PathEscape(row.Variant),
				url.PathEscape(row.Vulnerability), url.PathEscape(row.Component)),
			Private: private,
		}
		for personID, per := range acts {
			at := per[row.ProductID]
			if private && !at.private {
				continue
			}
			if !private && !at.public {
				continue
			}
			out[personID] = append(out[personID], holds)
		}
	}
	return out, nil
}

// pastDisclosure is every embargo whose date has arrived with nothing decided,
// against the people who should hear about it.
//
// **Administrators, and whoever holds it** (ACC-47). Nobody else: every one of
// these is a finding nobody has announced, so the alert is a disclosure in its
// own right, and the person holding it is told only where they may read
// undisclosed work in that product — an assignment that outlived the role that
// allowed it would otherwise deliver the thing the role was withdrawn to stop.
//
// It is derived rather than remembered, so an embargo somebody extends leaves
// this list on the next sweep without anybody dismissing anything, and one
// that is disclosed leaves it because the finding stops being private.
func (w *Watch) pastDisclosure(ctx context.Context, admins []int64) (map[int64][]Holds, error) {
	var rows []struct {
		Product       string    `bun:"product"`
		Stream        string    `bun:"stream"`
		Variant       string    `bun:"variant"`
		Component     string    `bun:"component"`
		Vulnerability string    `bun:"vulnerability"`
		ProductID     int64     `bun:"product_id"`
		DiscloseAt    time.Time `bun:"disclose_at"`
		AssignedTo    *int64    `bun:"assigned_to"`
	}
	err := w.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN product AS p ON p.id = st.product_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr("p.name AS product").
		ColumnExpr("st.name AS stream").
		ColumnExpr("va.name AS variant").
		ColumnExpr("c.name AS component").
		ColumnExpr("v.identifier AS vulnerability").
		ColumnExpr("st.product_id AS product_id").
		ColumnExpr("MIN(f.disclose_at) AS disclose_at").
		ColumnExpr("MIN(f.assigned_to) AS assigned_to").
		Where("f.visibility = ?", access.Private).
		Where("f.closed_at IS NULL").
		Where("f.disclose_at IS NOT NULL").
		Where("f.disclose_at <= ?", time.Now().UTC()).
		GroupExpr("p.name, st.name, va.name, c.name, v.identifier, st.product_id").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read what is past its disclosure date: %w", err)
	}
	// Note there is no early return for an empty answer. Every administrator
	// has to be handed a list either way, empty included: Reconcile makes
	// somebody's open set exactly what it is given, so an embargo that was
	// extended clears the alert it opened only because the next sweep hands
	// the same person a list without it. Returning nothing at all would leave
	// every cleared condition standing.
	people, held, err := access.NewStore(w.db).People(ctx)
	if err != nil {
		return nil, fmt.Errorf("read who may hear about this: %w", err)
	}
	private := map[int64]map[int64]bool{}
	for _, person := range people {
		reaches := map[int64]bool{}
		for _, grant := range held[person.ID] {
			if grant.Active && (grant.Role == access.PrivateRead || grant.Role == access.PrivateTriage) {
				reaches[grant.ProductID] = true
			}
		}
		private[person.ID] = reaches
	}

	out := make(map[int64][]Holds, len(admins)+len(rows))
	for _, admin := range admins {
		out[admin] = nil
	}
	// And everybody who is currently being told one of these, whether or not
	// they should still hear about anything. Reconcile makes one person's open
	// set exactly what it is handed, so somebody who is never handed a list is
	// never reconciled — and their alert stands after the thing it was about
	// has been answered.
	//
	// This does not arise for the conditions that only ever go to
	// administrators, because that set does not move. It arises here because
	// who hears about an embargo includes whoever holds it, and work is handed
	// around: the person who held it yesterday would keep an alert about a
	// date that has since been moved, with nothing left to clear it.
	told, err := w.beingTold(ctx, DisclosureDue)
	if err != nil {
		return nil, err
	}
	for _, person := range told {
		if _, already := out[person]; !already {
			out[person] = nil
		}
	}
	for _, row := range rows {
		where := row.Product + " " + row.Stream + " " + row.Variant + " " + row.Vulnerability
		holds := Holds{
			About: identify("disclosure " + where),
			Body: row.Vulnerability + " in " + row.Product + " reached its disclosure date on " +
				row.DiscloseAt.Format(time.DateOnly) + " and nothing has been decided.",
			Link: "/products/" + url.PathEscape(row.Product) +
				"/streams/" + url.PathEscape(row.Stream) +
				"/variants/" + url.PathEscape(row.Variant) +
				"/findings/" + url.PathEscape(row.Vulnerability) +
				"/components/" + url.PathEscape(row.Component),
			// Every one of these is about a finding nobody has announced —
			// that is what an embargo is — so what leaves this deployment
			// about it is a link and nothing else (NTF-15).
			Private: true,
		}
		for _, admin := range admins {
			out[admin] = append(out[admin], holds)
		}
		// And whoever holds it, where they may still read undisclosed work
		// here and are not already being told as an administrator.
		if row.AssignedTo == nil {
			continue
		}
		owner := *row.AssignedTo
		if _, already := out[owner]; already && contains(out[owner], holds.About) {
			continue
		}
		if !private[owner][row.ProductID] {
			continue
		}
		out[owner] = append(out[owner], holds)
	}
	return out, nil
}

// contains says a person is already being told about this one.
func contains(holding []Holds, about string) bool {
	for _, h := range holding {
		if h.About == about {
			return true
		}
	}
	return false
}

// Tidy clears what has run out: sessions past their lifetime.
//
// Sessions are the one table that grows with use rather than with what is
// tracked, and an expired one is refused on sight, so nothing but the table's
// size changes when they go. Nothing else here runs on a clock — the readers
// wait on work and the upstream pass on a setting — so the clearing rides on
// the sweep that already runs every quarter of an hour on every replica. Two
// replicas clearing at once delete the same rows or disjoint ones, and either
// way nothing is lost.
func (w *Watch) Tidy(ctx context.Context) (int64, error) {
	return access.NewStore(w.db).PurgeExpiredSessions(ctx)
}

// quietBuilds is every build nothing has been filed against for longer than
// this deployment allows.
//
// It reads the same answer the front page and the scans screen read, rather
// than asking a question of its own: two queries about when something was last
// scanned are two numbers that can disagree, and the one nobody is looking at
// would be the one that drifts.
func (w *Watch) quietBuilds(ctx context.Context) ([]Holds, error) {
	after, err := setting.NewStore(w.db).Duration(ctx, setting.QuietAfter, setting.DefaultQuietAfter)
	if err != nil {
		return nil, fmt.Errorf("read how long counts as quiet: %w", err)
	}

	// Asked as the deployment rather than as anybody in it, because that is
	// what this is: the tool reporting on itself rather than answering a
	// person. What it produces is then told only to administrators — who,
	// since ACC-64, do not themselves read every product, which is exactly
	// why this could no longer be an administrator's subject.
	everything := access.Everything("the watch")
	rows, err := ingest.NewStore(w.db).Scanning(ctx, everything, finding.Scope{}, after)
	if err != nil {
		return nil, err
	}

	holding := make([]Holds, 0, len(rows))
	for _, row := range rows {
		if !row.Quiet {
			continue
		}
		where := row.Product + " " + row.Stream + " " + row.Variant
		days := int(row.Since.Hours() / 24)
		body := fmt.Sprintf("%s has not been scanned for %d days. Nothing has failed — "+
			"nothing has arrived.", where, days)
		if row.LastReceivedAt == nil {
			body = fmt.Sprintf("%s was declared %d days ago and nothing has ever been "+
				"filed against it.", where, days)
		}
		holding = append(holding, Holds{
			// Hashed rather than the three names joined.
			//
			// Each of them may be 191 characters and the column holds 191, so
			// the obvious key does not fit — and what happens then depends on
			// the engine: three of them refuse the write and abort the sweep,
			// one truncates and silently collides. Joining them also collides
			// on its own, because a name may contain the separator: product
			// "a/b" branch "c" and product "a" branch "b/c" are one key.
			//
			// A hash is a fixed width, so it fits by construction, and it is
			// only ever compared for equality — nothing reads it back. The
			// names people read are in the body.
			About: identify(where),
			Body:  body,
			Link: "/products/" + url.PathEscape(row.Product) +
				"/streams/" + url.PathEscape(row.Stream) +
				"/variants/" + url.PathEscape(row.Variant) + "/scans",
		})
	}
	return holding, nil
}

// administrators is who hears about the tool's own health.
func (w *Watch) administrators(ctx context.Context) ([]int64, error) {
	people, _, err := access.NewStore(w.db).People(ctx)
	if err != nil {
		return nil, fmt.Errorf("read who administers this: %w", err)
	}
	var admins []int64
	for _, person := range people {
		if person.IsAdmin {
			admins = append(admins, person.ID)
		}
	}
	return admins, nil
}

// identify is the key a condition is recognized by between sweeps.
//
// Hashed for the reason every other identity here is: it is compared for
// equality and never read, and a fixed width fits a column whatever the names
// were.
func identify(what string) string {
	sum := sha256.Sum256([]byte(what))
	return hex.EncodeToString(sum[:])
}

// beingTold is everybody holding an open condition of one kind.
//
// The sweep has to hand each of them a list, even an empty one, or a condition
// that has stopped being true has nothing to clear it.
func (w *Watch) beingTold(ctx context.Context, kind Kind) ([]int64, error) {
	var people []int64
	err := w.db.NewSelect().
		TableExpr("notification AS n").
		ColumnExpr("n.person_id").
		Where("n.kind = ?", kind).
		Where("n.cleared_at IS NULL").
		GroupExpr("n.person_id").
		Scan(ctx, &people)
	if err != nil {
		return nil, fmt.Errorf("read who is being told about %s: %w", kind, err)
	}
	return people, nil
}

// holdingAbsent is everybody who has not signed in for a while and is still
// holding work (ACC-45).
//
// **Both halves, and the second is what makes it worth saying.** An account
// nobody has used in a month is harmless if it holds nothing; work stuck behind
// somebody who is not here is the problem, and this is the prompt that makes an
// administrator realize they have gone.
//
// **It asks rather than acts.** Long leave and having left look identical from
// here, and nothing detects somebody leaving (ACC-44) — so this opens a
// condition an administrator reads and clears by doing something, rather than
// handing the work back on its own. Withdrawing a role does hand work back
// (ACC-43), and that is a deliberate act by a person.
func (w *Watch) holdingAbsent(ctx context.Context) ([]Holds, error) {
	after, err := setting.NewStore(w.db).Duration(ctx,
		setting.AbsentAfter, setting.DefaultAbsentAfter)
	if err != nil {
		return nil, fmt.Errorf("read how long counts as absent: %w", err)
	}
	if after <= 0 {
		return nil, nil
	}
	since := time.Now().UTC().Add(-after)

	var rows []struct {
		ID         int64      `bun:"id"`
		Identity   string     `bun:"identity"`
		Name       string     `bun:"name"`
		LastSeenAt *time.Time `bun:"last_seen_at"`
		CreatedAt  time.Time  `bun:"created_at"`
		Holding    int        `bun:"holding"`
	}
	// The open findings each person holds, counted as pieces of work rather
	// than as rows: one issue in one component is one thing to deal with
	// however many places it sits at, and "6 items" meaning 288 findings is a
	// number that makes somebody's absence look like a catastrophe.
	held := w.db.NewSelect().
		Distinct().
		TableExpr("finding AS f").
		ColumnExpr("f.assigned_to AS person_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		Where("f.assigned_to IS NOT NULL").
		Where("f.closed_at IS NULL")

	if err := w.db.NewSelect().
		TableExpr("person AS p").
		Join("JOIN (?) AS work ON work.person_id = p.id", held).
		ColumnExpr("p.id AS id").
		ColumnExpr("p.identity AS identity").
		ColumnExpr("COALESCE(p.display_name, '') AS name").
		ColumnExpr("p.last_seen_at AS last_seen_at").
		ColumnExpr("p.created_at AS created_at").
		ColumnExpr("COUNT(*) AS holding").
		// Never signed in counts too — somebody granted a role and given work
		// who has not arrived is exactly the case worth raising — but it is
		// measured from when they were **added**, not from the beginning of
		// time. Compared against the moment alone, an administrator adding a
		// colleague and assigning them something raises an alert about them in
		// the same breath.
		//
		// The same shape as a build declared and never scanned, which is
		// measured from its declaration for exactly this reason.
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereOr("p.last_seen_at IS NOT NULL AND p.last_seen_at < ?", since).
				WhereOr("p.last_seen_at IS NULL AND p.created_at < ?", since)
		}).
		GroupExpr("p.id, p.identity, p.display_name, p.last_seen_at, p.created_at").
		OrderExpr("holding DESC, p.identity").
		Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read who is away and still holding work: %w", err)
	}

	holding := make([]Holds, 0, len(rows))
	for _, row := range rows {
		who := row.Name
		if who == "" {
			who = row.Identity
		}
		items := "items"
		if row.Holding == 1 {
			items = "item"
		}
		days := int(time.Since(row.CreatedAt).Hours() / 24)
		body := fmt.Sprintf("%s was added %d days ago, has never signed in, and has %d %s "+
			"assigned.", who, days, row.Holding, items)
		if row.LastSeenAt != nil {
			days = int(time.Since(*row.LastSeenAt).Hours() / 24)
			body = fmt.Sprintf("%s has not signed in for %d days and has %d %s assigned.",
				who, days, row.Holding, items)
		}
		holding = append(holding, Holds{
			// Keyed on the person rather than on the sentence, so the number
			// of items changing does not close one condition and open another
			// — an administrator would see the same person arrive in the list
			// every time somebody assigned them anything.
			About: identify("person:" + row.Identity),
			Body:  body,
			Link:  "/work?holder=" + url.QueryEscape(row.Identity),
		})
	}
	return holding, nil
}
