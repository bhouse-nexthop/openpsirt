package finding

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/currency"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// Open returns the findings currently open against a target that this subject
// may read.
//
// The filtering happens here rather than in whatever asked. A check in a
// handler is a check somebody forgets the first time they add another handler,
// and the thing being forgotten is not a blank screen — it is somebody seeing
// an issue that has not been disclosed.
//
// Which product the target belongs to is read here too, rather than accepted
// from the caller. A caller that could name the product could name a different
// one, and then the check would be answering a question nobody asked.
func (s *Store) Open(ctx context.Context, subject access.Subject, targetID int64) ([]Finding, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(productID) {
		// Not merely empty: a product somebody holds nothing on does not
		// exist as far as they are concerned, and an empty list is a
		// different statement from a refusal.
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []Finding
	err = s.db.NewSelect().Model(&rows).
		Where("target_id = ?", targetID).
		Where("closed_run_id IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Order("id").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read open findings: %w", err)
	}
	return rows, nil
}

// CountOpen counts what Open would return.
//
// A count is a read. Counting rows somebody may not see and reporting the
// total is the same disclosure as listing them, just compressed — and it is
// the path that leaks when only row reads are guarded.
func (s *Store) CountOpen(ctx context.Context, subject access.Subject, targetID int64) (int, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return 0, err
	}
	if !subject.Sees(productID) {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	n, err := s.db.NewSelect().Model((*Finding)(nil)).
		Where("target_id = ?", targetID).
		Where("closed_run_id IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count open findings: %w", err)
	}
	return n, nil
}

// visibleTo is what a subject may read in a product, as values to compare
// against. Empty means nothing, which is a refusal rather than a filter.
func visibleTo(subject access.Subject, productID int64) []access.Visibility {
	var visible []access.Visibility
	for _, v := range []access.Visibility{access.Public, access.Private} {
		if subject.Reads(v, productID) {
			visible = append(visible, v)
		}
	}
	return visible
}

// productOf reads which product a target belongs to.
func productOf(ctx context.Context, db bun.IDB, targetID int64) (int64, error) {
	var productID int64
	err := db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("st.product_id").
		Where("tg.id = ?", targetID).
		Scan(ctx, &productID)
	if err != nil {
		return 0, fmt.Errorf("look up which product target %d belongs to: %w", targetID, err)
	}
	return productID, nil
}

// Group is one issue in one component, with the places it occupies.
//
// This is the unit somebody decides about. The places are what the decision is
// recorded against — one real image produced 335,021 findings and 305,487 of
// them were a single kernel across the modules built against it, so a list of
// places is six thousand screens of rows differing in a column nobody reads.
type Group struct {
	Vulnerability string
	Severity      string
	Component     string
	Version       string
	// Upstream is what a fork was made from, where it is one. A version nobody
	// recognizes needs it to be explainable.
	Upstream string
	FixState FixState
	FixedIn  string
	// Places is how many consumers pull this component in here. It is part of
	// what is read rather than a detail: sixty-two places and one place are
	// different situations to somebody deciding, and a group that hides its
	// size invites a judgment made about one being applied to sixty-one
	// unseen.
	Places int
	// Answered counts the places the build has already argued about.
	Answered int
	// State is how far we have decided this group, in the same four words
	// the state filter takes and by the same definition: undecided when no
	// place has a decision of any kind, waiting when a claim stands proposed
	// and nobody has agreed, agreed when every place is answered by a
	// standing decision, lapsed when a decision here stopped applying and
	// nothing replaced it. Empty where none of the four holds — some places
	// approved and the rest never decided, with nothing waiting or lapsed.
	State string
	// SentBack says a live claim at one of the places is currently with its
	// author, which is the row the proposer is looking for in the list.
	SentBack bool
	// Urgency is how far up the list this belongs, and Exploited says whether
	// it is there because somebody is using it. The flag is carried rather
	// than left to be inferred from the number: a position nobody can explain
	// is one people stop trusting and then work around.
	Urgency   int64
	Exploited bool
	// LikelihoodPPM is the published estimate that this will be exploited, in
	// parts per million. Carried because it ranks *above* severity: without it
	// on the row, a medium sitting above a high looks like the list is
	// unsorted, when it is sorted by something the list never showed.
	LikelihoodPPM int
	// ScoreCenti is what the ordering actually compares. The severity word
	// beside it comes from whichever scoring generation the source used — a
	// 2003 issue scored 10.0 reads "high" under CVSS v2 and "critical" under
	// v3 — so a row showing only the word looks mis-sorted when two tie.
	ScoreCenti int
	// Owner and Parent are the two ends of the way down to this component: the
	// part of the product it belongs to, and what directly pulls it in
	// (UIX-12). Those two are what differ between sibling rows — the top says
	// which part of the product this is, the bottom is what a decision is
	// about — and the steps between them rarely distinguish anything, so
	// Middle counts them rather than naming them.
	//
	// A group covers every place the component sits at, and those places can
	// come down different ways. Chains says how many distinct ways there are,
	// so a row can say "one of 4" rather than presenting one route as though
	// it were the only one.
	Owner  string
	Parent string
	Middle int
	Chains int
}

// Severities in the order they rank, least first. A floor keeps this word and
// everything after it.
//
// A word outside this list — "negligible", "unknown", whatever a producer
// invents — ranks below all of them and so survives no floor at all. That is
// deliberate: a floor is a claim about how bad something is, and a rating
// nobody recognizes is not evidence of anything.
var severityOrder = []string{"low", "medium", "high", "critical"}

// Filter narrows what is open before it is paged.
//
// Narrowing belongs here rather than in whatever is displaying the result. A
// filter applied to a page that has already been fetched answers a different
// question from the one it appears to: "exploited" over fifty rows means
// exploited among those fifty, and paging with one on walks a different
// arbitrary subset each time. It also makes the total meaningless, which is the
// number people quote.
type Filter struct {
	// MinSeverity keeps issues rated at this word or worse. Empty — or "low",
	// which excludes nothing — keeps everything, including issues carrying no
	// rating at all.
	MinSeverity string
	// Exploited keeps only issues somebody is known to be exploiting.
	Exploited bool
	// HasFix keeps only issues where an upstream fixed version is known, which
	// is the set where the answer is to take a version rather than to judge.
	HasFix bool
	// Component keeps only what is open against components of this name.
	// Matched by name and not by version, because "what is wrong with openssl
	// here" is a question about the package rather than about one build of it,
	// and a build that vendors it twice should answer with both.
	Component string
	// Floor is what this product considers worth triaging, and BelowFloor
	// asks to see what it keeps out. The line is policy rather than
	// preference — somebody set it once for everybody — which is why what it
	// hides is counted and said rather than silently subtracted (TRI-43,
	// TRI-44).
	Floor      Floor
	BelowFloor bool
	// Search keeps components whose name contains this, without regard to
	// capitals. It is how somebody finds a package in a list of thousands,
	// where Component above is the exact name and answers a different
	// question: "show me openssl" against "show me anything ssl-ish".
	//
	// Matched here rather than in the browser for the reason every other
	// filter is: a search applied to the fifty rows already fetched searches
	// those fifty, and the total beside it would count something else.
	Search string
	// ProductID is which product the query narrows inside, set by the store
	// rather than by a caller. A place identity carries no product, so
	// anything correlating a place to a decision has to supply one or it
	// matches every product in the deployment.
	ProductID int64
	// Ecosystem keeps components of one package kind — deb, golang, python.
	// Read from the package identifier rather than stored beside it, because
	// the identifier is what says it and a second copy is a second thing to
	// keep true.
	//
	// It is the closest thing the data has to "userland and not the rest": a
	// kernel and its modules are Debian packages, a statically linked service
	// is Go, and somebody triaging one is usually not triaging the other.
	Ecosystem string
	// Under keeps what sits inside one container, by its name. UnderTheBuild
	// asks for the other case: what the build holds directly, which has no
	// consumer to name.
	Under         string
	UnderTheBuild bool
	// Beneath keeps what sits at a component or anywhere under it, by the
	// component identifiers a walk over the build's edges produced: the same
	// walk the dependency tree's cumulative count makes, so the number the
	// tree draws and the list it opens agree. Nil is no narrowing; empty is
	// nothing, which a component with no findings under it legitimately is.
	Beneath []int64
	// State keeps groups by how far they have been decided. A group is an
	// issue in a component across every place it sits, and its places can be
	// in different states, so what a group's state *is* had to be chosen:
	//
	//   undecided  no place has a decision of any kind
	//   waiting    a claim stands proposed at a place and nobody has agreed
	//   agreed     every place is answered by a standing decision
	//   lapsed     a decision here stopped applying and nothing replaced it
	//
	// Partly answered is deliberately not a state of its own: the row already
	// says "12 places · 3 answered", which is the more useful form of the same
	// fact, and a fifth word would be a filter for a number people can read.
	State string
	// Exclude drops components of these names.
	//
	// This exists because one package can drown the list. Measured on a switch
	// operating-system image: 4,943 of 6,822 rows — 72% — were the kernel, and
	// the next largest contributor had 58. Hiding it is not a preference about
	// tidiness, it is the difference between a list somebody reads and one they
	// scroll past. What makes it safe is that the total says so too: hiding is
	// narrowing, and narrowing is counted (REJ-10).
	Exclude []string
}

// severities returns the words a floor admits, or nil where it admits all of
// them and the filter should not be applied at all.
func (f Filter) severities() []string {
	for i, word := range severityOrder {
		if word == f.MinSeverity {
			if i == 0 {
				return nil
			}
			return severityOrder[i:]
		}
	}
	return nil
}

// narrow applies the filter to a grouped query over finding AS f, which must
// already join vulnerability AS v and component AS c.
//
// Severity is a condition on a row and the other two are conditions on the
// group, so they land in different clauses. Putting either in the other place
// is wrong rather than slow: a fix known at one place and not another would
// drop the places that lack it out of the count, and a group would report a
// size smaller than it is.
func (f Filter) narrow(q *bun.SelectQuery) *bun.SelectQuery {
	if words := f.severities(); len(words) > 0 {
		q = q.Where("v.severity IN (?)", bun.List(words))
	}
	if f.Exploited {
		q = q.Having("MAX(CASE WHEN f.urgency_exploited THEN 1 ELSE 0 END) = 1")
	}
	if f.HasFix {
		q = q.Having("MIN(f.fixed_in) IS NOT NULL AND MIN(f.fixed_in) <> ?", "")
	}
	if name := strings.TrimSpace(f.Component); name != "" {
		q = q.Where("c.name = ?", name)
	}
	// Lowered on both sides rather than asked to compare loosely: the engines
	// do not agree on what a case-insensitive comparison is, and one that is
	// spelled the same way everywhere behaves the same way everywhere (MDL-21).
	if term := strings.TrimSpace(f.Search); term != "" {
		q = q.Where("LOWER(c.name) LIKE ? ESCAPE '#'", "%"+contains(term)+"%")
	}
	if names := trimmed(f.Exclude); len(names) > 0 {
		q = q.Where("c.name NOT IN (?)", bun.List(names))
	}
	if eco := strings.TrimSpace(f.Ecosystem); eco != "" {
		q = q.Where("LOWER(c.purl) LIKE ? ESCAPE '#'", "pkg:"+contains(eco)+"/%")
	}
	// What holds it. A place records the component that pulls it in, so asking
	// what is inside a container is asking for places whose consumer is that
	// container — and what the build holds directly is the places with none.
	if f.UnderTheBuild {
		q = q.Where("f.consumer_id IS NULL")
	} else if under := strings.TrimSpace(f.Under); under != "" {
		q = q.Where("f.consumer_id IN (?)",
			q.NewSelect().TableExpr("component AS uc").
				Column("uc.id").Where("uc.name = ?", under))
	}
	if f.Beneath != nil {
		if len(f.Beneath) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("f.component_id IN (?)", bun.List(f.Beneath))
		}
	}
	q = f.byState(q)
	if !f.BelowFloor {
		q = f.Floor.narrow(q)
	}
	return q
}

// Hidden counts what the line keeps out of a list, so that the list can say so
// rather than showing a smaller number with nothing explaining it (TRI-44).
//
// Counted through the rest of the filter, because "hidden by the line" has to
// mean hidden from *this* list — a number counted against everything would say
// six thousand on a page showing fifty.
func (s *Store) Hidden(ctx context.Context, subject access.Subject, targetID int64,
	filter Filter) (int, error) {

	if !filter.Floor.Hides() || filter.BelowFloor {
		return 0, nil
	}
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return 0, err
	}
	visible := visibleTo(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	// The same query, with the line inverted rather than removed.
	below := filter
	below.Floor = Floor{}
	// The same reason as everywhere else this narrows: a place identity
	// carries no product, so the correlation has to be given one.
	below.ProductID = productID
	counted := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("f.vulnerability_id").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id")
	if words := filter.Floor.admits(); len(words) > 0 {
		counted = counted.Where("f.urgency_exploited = ?", false).
			Where(BandExpr+" NOT IN (?)", bun.List(words))
	}
	n, err := s.db.NewSelect().
		TableExpr("(?) AS grouped", below.narrow(counted)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count what the line keeps out: %w", err)
	}
	return n, nil
}

// stateWord says how far a group has been decided, from the same counts the
// state filter uses. The order is the filter's: a group every place of which
// is answered is agreed whatever else its history holds; one with a claim
// waiting is waiting; one where a decision lapsed and nothing stands is
// lapsed; one nobody ever decided about is undecided. Some places approved
// and the rest never decided, with nothing waiting or lapsed, is none of the
// four, and says so by saying nothing.
func stateWord(places, anyClaim, waiting, approved, lapsed int) string {
	switch {
	case places > 0 && approved == places:
		return "agreed"
	case waiting > 0:
		return "waiting"
	case lapsed > 0 && approved == 0:
		return "lapsed"
	case anyClaim == 0:
		return "undecided"
	}
	return ""
}

// byState keeps groups by how far they have been decided.
//
// Every one of these is a condition over the *group* rather than over a place,
// so they are HAVING clauses: a group is undecided when none of its places has
// a decision, not when one of them does not.
//
// **These read the decision table and nothing else.** The first version read
// `suppressed_by`, which is not a decision of ours at all: it points at a
// suppression, and a suppression is a claim the *build* made in its own scan
// file (only internal/sbom ever writes one). So "agreed" meant "the vendor's
// SBOM argued this away", a claim by a different author that nobody here
// reviewed — and a decision actually approved by a second person matched none
// of the four states. What the build argued away is a real number and the row
// already carries it separately, as how many places are answered; it is not
// how far *we* have decided.
//
// **Asked as a correlated subquery rather than a join**, for two reasons that
// would each decide it alone.
//
// A decision belongs to a product and the two keys linking one to a finding do
// not: an issue is one row per identifier for the whole deployment, and a
// place identity is a hash of a consumer and a component with no product in it
// (see PlaceIdentity — deliberately, so a place is recognized across
// variants). A join on those two alone therefore matches decisions in *every*
// product, which reports somebody else's triage as this product's state and
// tells a reader that a claim is pending in a product they cannot see.
//
// And a join multiplies. Every query this narrows also aggregates over finding
// rows — COUNT(*) for how many places, SUM for how many are answered — so one
// place carrying three historical decisions would report itself as three
// places, and the page would disagree with its own total.
func (f Filter) byState(q *bun.SelectQuery) *bun.SelectQuery {
	state := strings.TrimSpace(f.State)
	if state == "" {
		return q
	}
	// The words are spelled here rather than taken from the triage package,
	// which imports this one — naming them there and reading them here is a
	// cycle. They are the values stored in the column either way, and the
	// enum on the endpoint is what keeps a caller from inventing a fifth.
	const (
		proposed = "proposed"
		approved = "approved"
		lapsed   = "lapsed"
	)
	// One place, one answer: whether a claim of ours, in this product, at this
	// place, about this issue, is in the state being asked about.
	// Bound rather than spelled into the statement. These are two compile-time
	// constants and nothing a caller supplies, so there is nothing to inject —
	// but a value in a placeholder is the rule (SEC-01), and a literal here is
	// the shape somebody copies to a place where it does matter.
	at := func(live bool) string {
		clause := `SUM(CASE WHEN EXISTS (SELECT 1 FROM "decision" AS de
			WHERE de.product_id = ?
			  AND de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity
			  AND de.state = ?`
		if live {
			// Only the claim that currently stands. Without it a judgment
			// withdrawn eighteen months ago still answers for this place.
			clause += ` AND de.live_key IS NOT NULL`
		}
		return clause + `) THEN 1 ELSE 0 END)`
	}
	anyClaim := `SUM(CASE WHEN EXISTS (SELECT 1 FROM "decision" AS de
		WHERE de.product_id = ?
		  AND de.vulnerability_id = f.vulnerability_id
		  AND de.place_identity = f.place_identity) THEN 1 ELSE 0 END)`

	switch state {
	case "agreed":
		return q.Having(at(true)+" = COUNT(*)", f.ProductID, approved)
	case "waiting":
		return q.Having(at(false)+" > 0", f.ProductID, proposed)
	case "lapsed":
		return q.Having(at(false)+" > 0 AND "+at(true)+" = 0",
			f.ProductID, lapsed, f.ProductID, approved)
	case "undecided":
		return q.Having(anyClaim+" = 0", f.ProductID)
	}
	return q
}

// contains prepares a term to be searched for literally.
//
// A search box is not a pattern language. Typing `50%` means a component whose
// name contains "50%", not every component containing "50" — and `a_b` means
// what it says rather than "a, anything, b". So the wildcards are escaped and
// the escape character is stated: every engine here takes `ESCAPE`, and SQLite
// has no default escape character at all, so leaving it out makes a backslash
// mean one thing on three engines and another on the fourth.
//
// **The escape character is `#`, and a backslash is what it must not be.**
// MySQL and MariaDB treat a backslash as an escape inside a string literal, so
// `ESCAPE '\'` is an unterminated string: a syntax error there, and parsed
// happily by the other two. Caught by the four-engine run, which is the whole
// reason that run exists.
//
// **Case is folded here and again by the engine**, which is a compromise worth
// naming. MDL-21 says to normalize the stored value rather than ask an engine
// to compare loosely, and there is no folded column on a component to compare
// against — adding one is a migration and a backfill. Folding the term in Go
// is Unicode-aware; `LOWER()` on the column is ASCII-only on SQLite. So a
// component named with a non-ASCII capital is found on three engines and
// missed on the fourth. Component names are ASCII in every producer seen so
// far, which is why this is written down rather than fixed: the day that stops
// being true, the fix is a folded column.
func contains(term string) string {
	replacer := strings.NewReplacer("#", "##", "%", "#%", "_", "#_")
	return replacer.Replace(strings.ToLower(term))
}

// trimmed drops blanks from a list of names, so a stray separator in a query
// string does not become a name nothing matches.
func trimmed(names []string) []string {
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			kept = append(kept, name)
		}
	}
	return kept
}

// Groups returns what is open against a target, as the things somebody decides
// about rather than as one row per place.
func (s *Store) Groups(ctx context.Context, subject access.Subject, targetID int64, limit, offset int,
	filter Filter) ([]Group, int, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, 0, err
	}
	if !subject.Sees(productID) {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	// Which product this is narrowing inside. Set here rather than trusted
	// from the caller: a place identity carries no product, so a filter that
	// correlates one to a decision and is handed a zero would match every
	// product in the deployment.
	filter.ProductID = productID

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// Grouped in the database and named in a second pass. Reducing text across
	// rows has no portable spelling — the function differs on every engine —
	// and the counts are what this query is for.
	var rows []struct {
		VulnerabilityID int64  `bun:"vulnerability_id"`
		ComponentID     int64  `bun:"component_id"`
		Places          int    `bun:"places"`
		Answered        int    `bun:"answered"`
		Urgency         int64  `bun:"urgency"`
		Exploited       bool   `bun:"exploited"`
		LikelihoodPPM   int    `bun:"likelihood_ppm"`
		ScoreCenti      int    `bun:"score_centi"`
		FixState        string `bun:"fix_state"`
		FixedIn         string `bun:"fixed_in"`
		ConsumerID      *int64 `bun:"consumer_id"`
		Consumers       int    `bun:"consumers"`
		Direct          int    `bun:"direct"`
		AnyClaim        int    `bun:"any_claim"`
		Waiting         int    `bun:"waiting_here"`
		Approved        int    `bun:"approved_here"`
		Lapsed          int    `bun:"lapsed_here"`
		SentBack        int    `bun:"sent_back_here"`
	}
	// How far each group has been decided, counted the way the state filter
	// counts it and in the same statement, so the row and the filter cannot
	// disagree. Four correlated counts over our decisions in this product at
	// each place, plus whether any live claim is with its author.
	decided := func(alias, condition string) string {
		return `SUM(CASE WHEN EXISTS (SELECT 1 FROM "decision" AS de
			WHERE de.product_id = ?
			  AND de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity` + condition + `) THEN 1 ELSE 0 END) AS ` + alias
	}
	page := s.db.NewSelect().
		TableExpr("finding AS f").
		// Joined for the likelihood, and for the severity a floor compares. It
		// ranks above severity, so a list that orders by it and does not show
		// it looks unsorted.
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		// Joined so a filter can name a component. The names are still read in
		// a second pass, because reducing text across a group has no portable
		// spelling.
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("COUNT(*) AS places").
		// The most urgent place this issue sits at. A group is one decision
		// about one issue in one component, so what should decide where that
		// decision appears is the worst of what it covers.
		ColumnExpr("MAX(f.urgency) AS urgency").
		ColumnExpr("MAX(COALESCE(v.likelihood_ppm, 0)) AS likelihood_ppm").
		ColumnExpr("MAX(COALESCE(v.score_centi, 0)) AS score_centi").
		// Folded in Go from the same maximum rather than aggregated: no
		// portable spelling reduces a boolean across rows, and one engine
		// rejects the obvious one outright.
		ColumnExpr("MAX(CASE WHEN f.urgency_exploited THEN 1 ELSE 0 END) AS exploited").
		ColumnExpr("SUM(CASE WHEN f.suppressed_by IS NULL THEN 0 ELSE 1 END) AS answered").
		ColumnExpr("MIN(f.fix_state) AS fix_state").
		ColumnExpr("MIN(f.fixed_in) AS fixed_in").
		// One of the ways down, and how many there are. MIN passes over the
		// places the build pulls in directly, whose consumer is null, so those
		// are counted separately rather than being read as "no route at all".
		ColumnExpr("MIN(f.consumer_id) AS consumer_id").
		ColumnExpr("COUNT(DISTINCT f.consumer_id) AS consumers").
		ColumnExpr("SUM(CASE WHEN f.consumer_id IS NULL THEN 1 ELSE 0 END) AS direct").
		ColumnExpr(decided("any_claim", ""), productID).
		ColumnExpr(decided("waiting_here", " AND de.state = ? AND de.live_key IS NOT NULL"),
			productID, "proposed").
		ColumnExpr(decided("approved_here", " AND de.state = ? AND de.live_key IS NOT NULL"),
			productID, "approved").
		ColumnExpr(decided("lapsed_here", " AND de.state = ?"), productID, "lapsed").
		ColumnExpr(decided("sent_back_here",
			" AND de.state = ? AND de.live_key IS NOT NULL AND de.sent_back_at IS NOT NULL"),
			productID, "proposed").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id").
		// Ordered by urgency rather than by how widespread something is.
		// Sorting by place count puts whatever ships in the most places at the
		// top, which on a real image is the kernel — everywhere, and not
		// therefore the thing to look at first. What somebody with an hour
		// needs at the top is what is being exploited.
		OrderExpr("urgency DESC, places DESC, f.vulnerability_id, f.component_id").
		Limit(limit).Offset(offset)
	if err = filter.narrow(page).Scan(ctx, &rows); err != nil {
		return nil, 0, fmt.Errorf("read what is open: %w", err)
	}

	// Counted through the same filter as the page. A total that ignores the
	// narrowing is worse than no total: it reports how much there is to decide
	// about, which is the figure people quote, while the list beside it shows
	// something else.
	counted := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("f.vulnerability_id").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id")
	total, err := s.db.NewSelect().
		TableExpr("(?) AS grouped", filter.narrow(counted)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is open: %w", err)
	}

	// Named in two more queries rather than two per row. A page of fifty was
	// a hundred and one round trips, every one of them a primary-key lookup —
	// and this is the screen somebody opens first, against the largest product
	// they have.
	//
	// The failures are reported rather than skipped. Each lookup used to be
	// ignored when it failed, so a database in trouble produced a findings
	// list with blank component names in it: a page that looks like data and
	// is not.
	issues := make([]int64, 0, len(rows))
	components := make([]int64, 0, len(rows))
	for _, row := range rows {
		issues = append(issues, row.VulnerabilityID)
		components = append(components, row.ComponentID)
	}
	named, err := issuesNamed(ctx, s.db, issues)
	if err != nil {
		return nil, 0, err
	}
	shipped, err := componentsNamed(ctx, s.db, components)
	if err != nil {
		return nil, 0, err
	}

	// Both ends of the way down to each row, in one pass over the page rather
	// than a walk per row. A row whose places are pulled in directly by the
	// build is asked about by the component itself, which lands on the same
	// answer with one step in it.
	wanted := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ConsumerID != nil {
			wanted = append(wanted, *row.ConsumerID)
		} else {
			wanted = append(wanted, row.ComponentID)
		}
	}
	chains, err := graph.NewStore(s.db).Chains(ctx, subject, targetID, wanted)
	if err != nil {
		return nil, 0, err
	}

	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		group := Group{
			Places: row.Places, Answered: row.Answered,
			Urgency: row.Urgency, Exploited: row.Exploited,
			LikelihoodPPM: row.LikelihoodPPM, ScoreCenti: row.ScoreCenti,
			FixState: FixState(row.FixState), FixedIn: row.FixedIn,
			State:    stateWord(row.Places, row.AnyClaim, row.Waiting, row.Approved, row.Lapsed),
			SentBack: row.SentBack > 0,
		}
		if issue, held := named[row.VulnerabilityID]; held {
			group.Vulnerability, group.Severity = issue.Identifier, issue.Severity
		}
		if component, held := shipped[row.ComponentID]; held {
			group.Component, group.Version = component.Name, component.Version
			if component.UpstreamVersion != "" {
				group.Upstream = component.UpstreamName + " " + component.UpstreamVersion
			}
		}
		// How many distinct ways down there are: the consumers this component
		// has here, plus one for the build pulling it in directly.
		group.Chains = row.Consumers
		if row.Direct > 0 {
			group.Chains++
		}
		down := chains[row.ComponentID]
		if row.ConsumerID != nil {
			down = chains[*row.ConsumerID]
		}
		group.Owner, group.Parent, group.Middle = Ends(down)
		groups = append(groups, group)
	}
	return groups, total, nil
}

// ends reduces a way down to the two steps worth showing and a count of what
// was left out.
//
// The chain arrives build-first. The build itself is not one of the two: every
// row in a list scoped to one build shares it, so naming it in every row says
// nothing and costs the width that the parts which differ need.
// Ends returns the two ends of a way down and how many steps sit between
// them: the part of the product a component belongs to, and what directly
// pulls it in (UIX-12). Exported because a decision is described the same way
// wherever it is listed, and a second spelling of which step is which is how
// two screens disagree about where a component sits.
func Ends(down []graph.Step) (owner, parent string, middle int) {
	switch len(down) {
	case 0:
		// The inventory placed the component nowhere. Saying so is the honest
		// answer; claiming the product itself pulls it in is a comfortable
		// sentence and not a true one.
		return "", "", 0
	case 1:
		// The build pulls it in directly, so both ends are the build.
		return down[0].Name, down[0].Name, 0
	}
	owner = down[1].Name
	parent = down[len(down)-1].Name
	// Everything between the two named steps.
	if middle = len(down) - 3; middle < 0 {
		middle = 0
	}
	return owner, parent, middle
}

// issuesNamed reads what these issues are called and how bad they are said to be.
func issuesNamed(ctx context.Context, db *bun.DB, ids []int64) (map[int64]Vulnerability, error) {
	held := map[int64]Vulnerability{}
	if len(ids) == 0 {
		return held, nil
	}
	var issues []Vulnerability
	if err := db.NewSelect().Model(&issues).
		Column("id", "identifier", "severity").
		Where("id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what these issues are: %w", err)
	}
	for _, issue := range issues {
		held[issue.ID] = issue
	}
	return held, nil
}

// componentsNamed reads what these components are, including the upstream they
// were cut from where one is known.
func componentsNamed(ctx context.Context, db *bun.DB, ids []int64) (map[int64]graph.Component, error) {
	held := map[int64]graph.Component{}
	if len(ids) == 0 {
		return held, nil
	}
	var components []graph.Component
	if err := db.NewSelect().Model(&components).
		Column("id", "name", "version", "upstream_name", "upstream_version").
		Where("id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what these components are: %w", err)
	}
	for _, component := range components {
		held[component.ID] = component
	}
	return held, nil
}

// Evidence is everything held about one issue in one component here.
//
// Assembled for somebody who has to decide about it and has a thousand more
// waiting. The measure it is built against: nothing here should send them to a
// search engine. If we hold the write-up, the score, the patch, or the version
// that fixes it, it is in this answer.
type Evidence struct {
	Vulnerability string
	Aliases       []string
	Severity      string
	// ScoreCenti and Vector are the severity as a number and the statement of
	// what that number assumes. Network-reachable and unauthenticated is a
	// different judgment from local-and-privileged at the same score.
	ScoreCenti int
	Vector     string
	// Exploited and LikelihoodPPM are what separate the handful that matter
	// from the thousands that can wait.
	Exploited     bool
	LikelihoodPPM int
	Weaknesses    []string
	Description   string
	Advisory      string
	// References are everything the data points at, with patches told apart —
	// somebody deciding whether to backport rather than upgrade needs the
	// change itself, and hunting for it by hand is the step that does not
	// happen when a thousand findings are waiting.
	References []Reference

	// Assessed is what we say instead of the published rating, where somebody
	// has said something. Both are carried, because showing only ours would
	// read as the world's (TRI-42).
	Assessed  string
	Component string
	Version   string
	Upstream  string
	// FixState, FixedIn and FixedAt are what upstream has done about it, which
	// is the difference between "decide whether this matters" and "take the
	// next version".
	FixState FixState
	FixedIn  string
	FixedAt  *time.Time
	// ArrivedFrom is the version this place held before, where the version
	// moved and the issue came with it. Its presence says somebody bumped this
	// and the bump did not resolve it — which is aimed at whoever did the
	// bump rather than at whoever triages.
	ArrivedFrom string

	// What the ecosystem's own index says is newest, and when it shipped
	// (ING-41). Empty where asking is turned off, where nothing has asked yet,
	// and where the index has never heard of the component.
	LatestVersion    string
	LatestReleasedAt *time.Time
	// NothingSince says upstream has shipped nothing since the year this issue
	// was named, and there is no fix. Two dates compared, not a judgment about
	// anybody's project: it is the reason there is no fix rather than a claim
	// that the project is dead.
	NothingSince bool
	// Places is where it sits here — the consumer that pulls the component in,
	// and whether the build has already argued that place away.
	Places []Sitting
}

// Sitting is one place a component occupies, as a finding presents it.
type Sitting struct {
	// PlaceIdentity is what a decision is made against, and what a request
	// names when making one.
	PlaceIdentity string
	Consumer      string
	Suppressed    bool
	Urgency       int64
	// Decision is the claim already standing here, where one does. Once one judgment can
	// cover a chosen subset of places (TRI-37), a finding half answered has to
	// look different from one nobody has touched — otherwise the places left
	// open are invisible and somebody answers them twice or not at all
	// (UIX-40).
	//
	// Not the same as Suppressed, which is the build's own argument in its VEX
	// documents: a different claim by a different author.
	Decision *int64
	// Chain is the way down to here, the build first and this component last,
	// with a version at every step.
	//
	// The direct consumer is what a decision is keyed on (MDL-06) and it is
	// not enough to *read*: where a component is reached several ways the
	// consumer is often the same word twice, and two identical rows do not
	// distinguish two places. `UIX-14` asks for the whole chain for that
	// reason. It stays display-only — putting it back into identity is what
	// MDL-06 measured and rejected, at 49,170 paths against 48 consumers.
	//
	// Empty where the build's inventory left this component unplaced, which is
	// a real state and not an error: a producer that names no root, or a
	// component nothing was recorded as pulling in.
	Chain []graph.Step
}

// Detail reads everything held about one issue in one component of a build.
func (s *Store) Detail(ctx context.Context, subject access.Subject, targetID, vulnerabilityID,
	componentID int64) (*Evidence, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	visible := visibleTo(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []struct {
		PlaceIdentity string     `bun:"place_identity"`
		Consumer      string     `bun:"consumer"`
		ConsumerID    *int64     `bun:"consumer_id"`
		Decision      *int64     `bun:"decision"`
		Suppressed    bool       `bun:"suppressed"`
		Urgency       int64      `bun:"urgency"`
		FixState      string     `bun:"fix_state"`
		FixedIn       string     `bun:"fixed_in"`
		FixedAt       *time.Time `bun:"fixed_at"`
		ArrivedFrom   string     `bun:"arrived_from"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.place_identity AS place_identity").
		ColumnExpr("COALESCE(uc.name, '') AS consumer").
		ColumnExpr("f.consumer_id AS consumer_id").
		// A live claim standing at this place, whatever its state. Proposed
		// and waiting counts: it is answered as far as the person looking at
		// it is concerned, and showing it as untouched invites a second claim
		// about the same code.
		ColumnExpr(`(SELECT MIN(de.id) FROM "decision" AS de
			WHERE de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity
			  AND de.live_key IS NOT NULL) AS decision`).
		ColumnExpr("CASE WHEN f.suppressed_by IS NULL THEN ? ELSE ? END AS suppressed", false, true).
		ColumnExpr("f.urgency AS urgency").
		ColumnExpr("f.fix_state AS fix_state").
		ColumnExpr("f.fixed_in AS fixed_in").
		ColumnExpr("f.fixed_at AS fixed_at").
		ColumnExpr("COALESCE(f.arrived_from, '') AS arrived_from").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.component_id = ?", componentID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		OrderExpr("f.urgency DESC, consumer").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read where this sits: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no open finding is recorded there")
	}

	var issue Vulnerability
	if err := s.db.NewSelect().Model(&issue).Where("id = ?", vulnerabilityID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what this issue is: %w", err)
	}
	var component graph.Component
	if err := s.db.NewSelect().Model(&component).Where("id = ?", componentID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what this component is: %w", err)
	}

	var aliases []Alias
	if err := s.db.NewSelect().Model(&aliases).
		Where("vulnerability_id = ?", vulnerabilityID).
		Order("identifier").Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what else this issue is called: %w", err)
	}
	var references []Reference
	if err := s.db.NewSelect().Model(&references).
		Where("vulnerability_id = ?", vulnerabilityID).
		// Patches first: for somebody deciding whether to backport, the change
		// itself is the answer and everything else is background.
		OrderExpr("CASE WHEN kind = ? THEN 0 ELSE 1 END, url", Patch).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("read where this is written up: %w", err)
	}

	evidence := &Evidence{
		Vulnerability: issue.Identifier, Severity: issue.Severity,
		Assessed: func() string {
			if issue.AssessedSeverity == nil {
				return ""
			}
			return *issue.AssessedSeverity
		}(),
		Vector: issue.Vector, Exploited: issue.Exploited,
		Description: issue.Description, Advisory: issue.Advisory,
		References: references,
		Component:  component.Name, Version: component.Version,
		FixState: FixState(rows[0].FixState), FixedIn: rows[0].FixedIn, FixedAt: rows[0].FixedAt,
		ArrivedFrom: rows[0].ArrivedFrom,
	}
	if component.LatestVersion != nil {
		evidence.LatestVersion = *component.LatestVersion
	}
	evidence.LatestReleasedAt = component.LatestReleasedAt
	if component.LatestReleasedAt != nil && FixState(rows[0].FixState) != FixedUpstream {
		evidence.NothingSince = currency.NothingSince(
			issue.Identifier, *component.LatestReleasedAt)
	}
	if issue.ScoreCenti != nil {
		evidence.ScoreCenti = *issue.ScoreCenti
	}
	if issue.LikelihoodPPM != nil {
		evidence.LikelihoodPPM = *issue.LikelihoodPPM
	}
	if issue.Weaknesses != "" {
		evidence.Weaknesses = strings.Split(issue.Weaknesses, ",")
	}
	if component.UpstreamVersion != "" {
		evidence.Upstream = component.UpstreamName + " " + component.UpstreamVersion
	}
	for _, alias := range aliases {
		if alias.Identifier != issue.Identifier {
			evidence.Aliases = append(evidence.Aliases, alias.Identifier)
		}
	}
	// The way down to each place, read in one pass. A place whose consumer is
	// the build itself is asked about by the component, which lands on the
	// same answer with one step in it.
	wanted := make([]int64, 0, len(rows)+1)
	wanted = append(wanted, componentID)
	for _, row := range rows {
		if row.ConsumerID != nil {
			wanted = append(wanted, *row.ConsumerID)
		}
	}
	chains, err := graph.NewStore(s.db).Chains(ctx, subject, targetID, wanted)
	if err != nil {
		return nil, err
	}
	here := graph.Step{Name: component.Name, Version: component.Version}

	for _, row := range rows {
		place := Sitting{
			PlaceIdentity: row.PlaceIdentity, Consumer: row.Consumer,
			Suppressed: row.Suppressed, Decision: row.Decision, Urgency: row.Urgency,
		}
		// Down to the consumer, then this component under it. Where the build
		// pulls the component in directly there is no consumer, and the chain
		// down to the component itself is the whole of the answer.
		if row.ConsumerID != nil {
			if down, ok := chains[*row.ConsumerID]; ok && len(down) > 0 {
				place.Chain = append(append([]graph.Step{}, down...), here)
			}
		} else if down, ok := chains[componentID]; ok && len(down) > 0 {
			place.Chain = append([]graph.Step{}, down...)
		}
		evidence.Places = append(evidence.Places, place)
	}
	return evidence, nil
}

// AtComponent lists the open issues against one component of a build, with
// every place each one occupies.
//
// Every place, because a decision is keyed on one — so a claim built from a
// single arbitrary place would silence one consumer and leave the others open
// while reporting that it had covered them.
//
// The set somebody narrows before claiming something about all of it. What
// narrows it is theirs — a text match on what a report says is how a candidate
// is found, never why a claim is true.
func (s *Store) AtComponent(ctx context.Context, subject access.Subject, targetID,
	componentID int64, contains string, limit, offset int) ([]Deciding, int, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, 0, err
	}
	visible := visibleTo(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	narrow := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = q.TableExpr("finding AS f").
			Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
			Where("f.target_id = ?", targetID).
			Where("f.component_id = ?", componentID).
			Where("f.closed_run_id IS NULL").
			Where("f.visibility IN (?)", bun.List(visible))
		if contains != "" {
			// Matched against what a report says, which is all that is held
			// about where a flaw lives. Nothing here knows a kernel from a
			// font library.
			q = q.Where("LOWER(v.description) LIKE ?", "%"+strings.ToLower(contains)+"%")
		}
		return q
	}

	// Grouped and counted, not COUNT DISTINCT: with no GROUP BY, Count() emits
	// its own count(*) and the expression never reaches the statement, so the
	// total counted places while the list counts issues.
	total, err := s.db.NewSelect().
		TableExpr(`(?) AS "grouped"`, narrow(s.db.NewSelect()).
			ColumnExpr("f.vulnerability_id").GroupExpr("f.vulnerability_id")).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is open against this component: %w", err)
	}

	var rows []struct {
		VulnerabilityID   int64  `bun:"vulnerability_id"`
		PlaceIdentity     string `bun:"place_identity"`
		Visibility        string `bun:"visibility"`
		ComponentUpstream string `bun:"component_upstream"`
		ConsumerUpstream  string `bun:"consumer_upstream"`
		Severity          int    `bun:"severity_centi"`
		FixedIn           string `bun:"fixed_in"`
		Places            int    `bun:"places"`
	}
	err = narrow(s.db.NewSelect()).
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("MIN(f.place_identity) AS place_identity").
		// The most restrictive visibility any place of this issue has. MIN
		// sorts 'private' before 'public' alphabetically, which happens to be
		// the safe direction — stated here so it is a rule rather than an
		// accident somebody normalizes away.
		ColumnExpr("MIN(f.visibility) AS visibility").
		ColumnExpr("MIN("+ComponentUpstreamExpr+") AS component_upstream").
		ColumnExpr("MIN("+ConsumerUpstreamExpr+") AS consumer_upstream").
		ColumnExpr("MIN(COALESCE(v.score_centi, 0)) AS severity_centi").
		ColumnExpr("MIN(COALESCE(f.fixed_in, '')) AS fixed_in").
		ColumnExpr("COUNT(*) AS places").
		GroupExpr("f.vulnerability_id").
		OrderExpr("MAX(f.urgency) DESC, f.vulnerability_id").
		Limit(limit).Offset(offset).
		Scan(ctx, &rows)
	if err != nil {
		return nil, 0, fmt.Errorf("read what is open against this component: %w", err)
	}

	// Every place, fetched for the page of issues rather than one arbitrary
	// place per issue. A decision is keyed on a place, so a claim built from
	// MIN(place_identity) covers one consumer and leaves the rest open while
	// reporting that it covered them.
	issues := make([]int64, 0, len(rows))
	for _, row := range rows {
		issues = append(issues, row.VulnerabilityID)
	}
	everywhere, err := s.placesOf(ctx, targetID, componentID, issues, visible)
	if err != nil {
		return nil, 0, err
	}

	at := make([]Deciding, 0, len(rows))
	for _, row := range rows {
		for _, place := range everywhere[row.VulnerabilityID] {
			place.ProductID = productID
			place.VulnerabilityID = row.VulnerabilityID
			place.SeverityCenti = row.Severity
			place.FixedIn = row.FixedIn
			place.Places = row.Places
			at = append(at, place)
		}
	}
	return at, total, nil
}

// placesOf reads every place a set of issues occupies at one component.
func (s *Store) placesOf(ctx context.Context, targetID, componentID int64, issues []int64,
	visible []access.Visibility) (map[int64][]Deciding, error) {

	everywhere := map[int64][]Deciding{}
	if len(issues) == 0 {
		return everywhere, nil
	}
	var rows []struct {
		VulnerabilityID   int64  `bun:"vulnerability_id"`
		PlaceIdentity     string `bun:"place_identity"`
		Visibility        string `bun:"visibility"`
		ComponentUpstream string `bun:"component_upstream"`
		ConsumerUpstream  string `bun:"consumer_upstream"`
	}
	err := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.place_identity AS place_identity").
		ColumnExpr("f.visibility AS visibility").
		ColumnExpr(ComponentUpstreamExpr+" AS component_upstream").
		ColumnExpr(ConsumerUpstreamExpr+" AS consumer_upstream").
		Where("f.target_id = ?", targetID).
		Where("f.component_id = ?", componentID).
		Where("f.closed_run_id IS NULL").
		Where("f.vulnerability_id IN (?)", bun.List(issues)).
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.place_identity, f.visibility, c.upstream_version, c.version, uc.upstream_version, uc.version").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read where these sit: %w", err)
	}
	for _, row := range rows {
		everywhere[row.VulnerabilityID] = append(everywhere[row.VulnerabilityID], Deciding{
			PlaceIdentity:     row.PlaceIdentity,
			Visibility:        access.AsVisibility(row.Visibility),
			ComponentUpstream: row.ComponentUpstream, ConsumerUpstream: row.ConsumerUpstream,
		})
	}
	return everywhere, nil
}

// ComponentGroup is one component at one version, with what is open against
// it counted rather than listed.
//
// The level above a findings list. A list of issues answers "what is wrong";
// this answers "where is the weight", which is the question somebody asks
// before deciding what to read and what to put aside. It is also how a person
// finds the one package worth hiding: on a real image the kernel carried 4,943
// of 6,822 rows, and no list of issues makes that visible — it just looks like
// a long list.
type ComponentGroup struct {
	Component string
	Version   string
	// Upstream is what a fork was cut from, carried for the same reason it is
	// carried on a finding: a version nobody recognizes needs it.
	Upstream string
	// Issues is how many distinct vulnerabilities are open against it, which
	// is how many rows it contributes to the findings list.
	Issues int
	// Places is how many times those sit somewhere in the build. The two
	// differ by orders of magnitude on shared code and the gap is the point:
	// one kernel issue reaching four hundred modules is one decision.
	Places int
	// Exploited says whether any of them is known-exploited, which is what
	// stops a component being put aside on the strength of its size alone.
	Exploited bool
	Urgency   int64
}

// ComponentGroups returns what is open against a target, gathered by the
// component it is open against rather than by the issue.
func (s *Store) ComponentGroups(ctx context.Context, subject access.Subject, targetID int64,
	limit, offset int, filter Filter) ([]ComponentGroup, int, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, 0, err
	}
	if !subject.Sees(productID) {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	filter.ProductID = productID
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var rows []struct {
		ComponentID int64 `bun:"component_id"`
		Issues      int   `bun:"issues"`
		Places      int   `bun:"places"`
		Exploited   bool  `bun:"exploited"`
		Urgency     int64 `bun:"urgency"`
	}
	page := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("f.component_id AS component_id").
		// Distinct issues rather than rows, because that is what the findings
		// list shows and therefore what hiding this component would remove
		// from it.
		ColumnExpr("COUNT(DISTINCT f.vulnerability_id) AS issues").
		ColumnExpr("COUNT(*) AS places").
		ColumnExpr("MAX(CASE WHEN f.urgency_exploited THEN 1 ELSE 0 END) AS exploited").
		ColumnExpr("MAX(f.urgency) AS urgency").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.component_id").
		// By weight, not by urgency. The question this view answers is where
		// the volume is, and ordering it by urgency would reproduce the
		// findings list with worse resolution.
		OrderExpr("issues DESC, places DESC, f.component_id").
		Limit(limit).Offset(offset)
	if err = filter.narrow(page).Scan(ctx, &rows); err != nil {
		return nil, 0, fmt.Errorf("read what is open by component: %w", err)
	}

	counted := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("f.component_id").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.component_id")
	total, err := s.db.NewSelect().
		TableExpr("(?) AS grouped", filter.narrow(counted)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is open by component: %w", err)
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ComponentID)
	}
	shipped, err := componentsNamed(ctx, s.db, ids)
	if err != nil {
		return nil, 0, err
	}

	groups := make([]ComponentGroup, 0, len(rows))
	for _, row := range rows {
		group := ComponentGroup{
			Issues: row.Issues, Places: row.Places,
			Exploited: row.Exploited, Urgency: row.Urgency,
		}
		if named, ok := shipped[row.ComponentID]; ok {
			group.Component = named.Name
			group.Version = named.Version
			group.Upstream = named.UpstreamVersion
		}
		groups = append(groups, group)
	}
	return groups, total, nil
}
