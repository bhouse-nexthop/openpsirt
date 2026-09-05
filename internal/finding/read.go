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

	visible := access.Visible(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []Finding
	err = s.db.NewSelect().Model(&rows).
		Where("target_id = ?", targetID).
		Where("closed_at IS NULL").
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

	visible := access.Visible(subject, productID)
	if len(visible) == 0 {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	n, err := s.db.NewSelect().Model((*Finding)(nil)).
		Where("target_id = ?", targetID).
		Where("closed_at IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count open findings: %w", err)
	}
	return n, nil
}

// visibleTo is what a subject may read in a product, as values to compare
// against. Empty means nothing, which is a refusal rather than a filter.
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
	// Ecosystem is the kind of package this is, as its identifier spells it.
	// Part of what tells one row from another: a name at a version is not
	// unique within a build, which can hold a source repository and the
	// package built from it under one name at one version.
	Ecosystem string
	FixState  FixState
	FixedIn   string
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
	//
	// All four are empty where the selection spans more than one build
	// (UIX-53). A chain belongs to one build's graph, and a group that sits in
	// three builds is reached three ways, so filling these from whichever
	// build the row happened to name would present one route as the answer —
	// the thing the paragraph above says not to do, one level up.
	Owner  string
	Parent string
	Middle int
	Chains int
	// Builds is how many builds in the selection hold this group, and Stream
	// and Variant name **one** of them — not the only one. A screen needs
	// somewhere to link to and an action needs a build to name; what says
	// there are others is the count, so a screen can show that instead of
	// reading the named one as the whole answer. Both are empty where the
	// selection is a single build, since the row would only be repeating it.
	Builds  int
	Stream  string
	Variant string
	// Matched says how the scanner reached this. One answer for the whole
	// group: every place of it comes from the same line of a report. Empty
	// where the scanner said nothing.
	Matched Matched `bun:"matched"`
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
	// Search keeps rows whose component name **or issue name** contains this,
	// without regard to capitals. It is how somebody finds a package in a list
	// of thousands, where Component above is the exact name and answers a
	// different question: "show me openssl" against "show me anything
	// ssl-ish".
	//
	// **The issue half is what an advisory landing actually asks for.** The
	// first question a PSIRT is asked is "where is CVE-2026-9079 in what we
	// ship", and matching component names alone answered nothing at all for
	// it — the box said it searched issues and returned an empty list, which
	// reads as "we do not ship it".
	//
	// Aliases count. An issue is one thing under several names (MDL-19), so
	// searching the name a reporter used has to reach the row filed under the
	// name a scanner used, or the answer depends on which feed got there
	// first.
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
	// component's identifier: the same walk over the build's edges the
	// dependency tree's cumulative count makes, so the number the tree draws
	// and the list it opens agree. Nil is no narrowing.
	Beneath *int64
	// TargetID is which build the query is over, set by the store rather than
	// by a caller, for the same reason ProductID is: the walk beneath a
	// component is a walk over one build's edges.
	TargetID int64
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
	// Outcome keeps groups a standing decision of this kind covers — the way
	// to ask "what have we dismissed", which State cannot answer: "agreed"
	// says a judgment stands, not which judgment. Read against what stands
	// now, so a dismissal withdrawn last year does not answer for its place.
	Outcome string
	// Assigned keeps groups by who is dealing with them: "me", "somebody" or
	// "nobody". A findings list that cannot say "mine" makes somebody go to a
	// second screen to find the work they already know is theirs.
	Assigned string
	// HeldBy is who "me" means, set by the store from the subject rather than
	// by a caller — for the same reason ProductID is: a caller supplying it
	// could ask for somebody else's while saying "mine".
	HeldBy int64
	// Reassessed keeps groups whose issue we rated differently from the
	// world. It is how somebody finds what has been re-prioritized here,
	// which is a question auditors ask and nothing else answers.
	Reassessed bool
	// Unconfirmed keeps groups a scanner reached only by comparing a published
	// identifier against an upstream version range, never against an advisory
	// for the package in its own ecosystem.
	//
	// This is the question somebody asks about a distribution's packages and
	// nothing else answers: a distribution backports fixes without moving the
	// upstream version, so busybox 1.37.0-r14 and 1.37.0-r15 are the same
	// release to an upstream range and one of them may carry the patch. These
	// are neither confirmed nor refuted — somebody has to look — and finding
	// them one at a time is not a thing anybody does.
	Unconfirmed bool
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

// narrow applies the filter to a grouped query over finding AS f.
//
// Nothing here needs vulnerability or component joined. Every condition on
// what an issue is rated or what a component is called is asked as a
// membership test against the table that holds the answer — "issues rated at
// least high", "components named openssl" — so the query being narrowed can
// walk finding's covering index and touch nothing else. The joins were the
// first version, and they were what put a row lookup behind every one of a
// build's open findings on every page: the engine had to read the row to
// find the key to join on, whether or not any filter used the joined table.
//
// Severity is a condition on a row and the fix is a condition on the group,
// so they land in different clauses. Putting either in the other place is
// wrong rather than slow: a fix known at one place and not another would drop
// the places that lack it out of the count, and a group would report a size
// smaller than it is.
func (f Filter) narrow(q *bun.SelectQuery) *bun.SelectQuery {
	if words := f.severities(); len(words) > 0 {
		q = q.Where("f.vulnerability_id IN (?)",
			q.NewSelect().TableExpr("vulnerability AS v").Column("v.id").
				Where("v.severity IN (?)", bun.List(words)))
	}
	if f.Exploited {
		// Read off the urgency rather than the flag beside it. A place known
		// to be exploited ranks in a band of its own above everything else
		// (Ranked.Rank), and the urgency is in the index the grouping walks
		// where the flag is not.
		q = q.Having("MAX(f.urgency) >= ?", int64(exploitedBand))
	}
	if f.HasFix {
		q = q.Having("MIN(f.fixed_in) IS NOT NULL AND MIN(f.fixed_in) <> ?", "")
	}
	if f.Unconfirmed {
		// One place answers for the group. A group is an issue at a component,
		// every place of it comes from the same line of a scanner's report,
		// and the applier writes that line to all of them — so they cannot
		// disagree, and an aggregate guarding against it would be a condition
		// nothing can make false.
		q = q.Having("MIN(f.matched) = ?", ByIdentifier)
	}
	if name := strings.TrimSpace(f.Component); name != "" {
		q = q.Where("f.component_id IN (?)", componentsWhere(q, "c.name = ?", name))
	}
	// Lowered on both sides rather than asked to compare loosely: the engines
	// do not agree on what a case-insensitive comparison is, and one that is
	// spelled the same way everywhere behaves the same way everywhere (MDL-21).
	if term := strings.TrimSpace(f.Search); term != "" {
		like := "%" + contains(term) + "%"
		// Either side matches. A person typing into one box does not say
		// which of the two they mean, and an identifier cannot be mistaken
		// for a package name in practice.
		q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				WhereOr("f.component_id IN (?)",
					componentsWhere(q, "LOWER(c.name) LIKE ? ESCAPE '#'", like)).
				WhereOr("f.vulnerability_id IN (?)",
					q.NewSelect().TableExpr("vulnerability AS v").Column("v.id").
						Where("LOWER(v.identifier) LIKE ? ESCAPE '#'", like)).
				WhereOr("f.vulnerability_id IN (?)",
					q.NewSelect().TableExpr("vulnerability_alias AS va").
						Column("va.vulnerability_id").
						Where("LOWER(va.identifier) LIKE ? ESCAPE '#'", like))
		})
	}
	if names := trimmed(f.Exclude); len(names) > 0 {
		q = q.Where("f.component_id NOT IN (?)",
			componentsWhere(q, "c.name IN (?)", bun.List(names)))
	}
	if eco := strings.TrimSpace(f.Ecosystem); eco != "" {
		q = q.Where("f.component_id IN (?)",
			componentsWhere(q, "LOWER(c.purl) LIKE ? ESCAPE '#'", "pkg:"+contains(eco)+"/%"))
	}
	// What holds it. A place records the component that pulls it in, so asking
	// what is inside a container is asking for places whose consumer is that
	// container — and what the build holds directly is the places with none.
	if f.UnderTheBuild {
		q = q.Where("f.consumer_id IS NULL")
	} else if under := strings.TrimSpace(f.Under); under != "" {
		q = q.Where("f.consumer_id IN (?)", componentsWhere(q, "c.name = ?", under))
	}
	// Who is dealing with it. Set for the whole group at once, so a group is
	// held when its places are — asked as MIN and MAX rather than as one row,
	// because a group whose places disagree is not "mine" and saying so would
	// hand somebody work that is half theirs.
	switch strings.TrimSpace(f.Assigned) {
	case "nobody":
		q = q.Having("COUNT(f.assigned_to) = 0")
	case "somebody":
		q = q.Having("COUNT(f.assigned_to) = COUNT(*)")
	case "me":
		if f.HeldBy != 0 {
			q = q.Having("COUNT(f.assigned_to) = COUNT(*)").
				Having("MIN(f.assigned_to) = ? AND MAX(f.assigned_to) = ?", f.HeldBy, f.HeldBy)
		}
	}
	// What we said about the issue, as against what was published. A rating of
	// ours is the record of a priority somebody changed here.
	if f.Reassessed {
		q = q.Where("f.vulnerability_id IN (?)",
			q.NewSelect().TableExpr("vulnerability AS v").Column("v.id").
				Where("v.assessed_severity IS NOT NULL"))
	}
	if f.Beneath != nil {
		// The subtree as the engine walks it, not as a list of identifiers
		// bound back in: under a build's root that list is the whole build.
		q = q.Where("f.component_id IN (?)", graph.Within(q.DB(), f.TargetID, *f.Beneath))
	}
	q = f.byState(q)
	if !f.BelowFloor {
		q = f.Floor.narrow(q)
	}
	return q
}

// componentsWhere is the identifiers of the components a condition selects,
// as a subquery for a membership test on a finding's component or consumer.
func componentsWhere(q *bun.SelectQuery, condition string, args ...any) *bun.SelectQuery {
	return q.NewSelect().TableExpr("component AS c").Column("c.id").Where(condition, args...)
}

// Hidden counts what the line keeps out of a list, so that the list can say so
// rather than showing a smaller number with nothing explaining it (TRI-44).
//
// Counted through the rest of the filter, because "hidden by the line" has to
// mean hidden from *this* list — a number counted against everything would say
// six thousand on a page showing fifty.
func (s *Store) Hidden(ctx context.Context, subject access.Subject, scope Scope,
	filter Filter) (int, error) {

	if !filter.Floor.Hides() || filter.BelowFloor {
		return 0, nil
	}
	// The same query, with the line inverted rather than removed.
	below := filter
	below.Floor = Floor{}
	_, visible, targets, err := s.inScope(ctx, subject, scope, &below)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	counted := s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.vulnerability_id").
		Where("f.target_id IN (?)", bun.List(targets)).
		Where("f.closed_at IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id")
	if words := filter.Floor.admits(); len(words) > 0 {
		// The line's own condition, negated: not exploited, and rated
		// beneath the line. Both read the way Floor.narrow reads them.
		counted = counted.Where("f.urgency < ?", int64(exploitedBand)).
			Where("f.vulnerability_id IN (?)",
				counted.NewSelect().TableExpr("vulnerability AS v").Column("v.id").
					Where(BandExpr+" NOT IN (?)", bun.List(words)))
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

// keyMatches says a decision row `de` was keyed on the versions a finding's
// component `c` and consumer `uc` hold now, as the versions are compared
// everywhere a decision is looked up for a finding. A decision stores no
// version where nothing stated one; the finding's expression reads that as
// empty, so the two absences are compared as the same absence.
//
// The three aliases are the caller's to join: a finding's own row carries
// only the identifiers, and the versions are on the components.
const keyMatches = "COALESCE(de.component_upstream_version, '') = " + ComponentUpstreamExpr +
	" AND COALESCE(de.consumer_upstream_version, '') = " + ConsumerUpstreamExpr

// coversHere says a decision row `de` is about the finding it was correlated
// with by issue and place, for the counts behind the state a row carries and
// the state filter that finds it.
//
// A live claim covers a place at the versions it was keyed on and no other:
// matched by place alone, a claim approved against libnl 3.7.0 in one build
// answered for libnl 3.9.0 at the same place in the next, while everything
// that asks whether a decision actually applies said it covered nothing there.
// A claim with no key has lapsed or been withdrawn, and by definition its
// versions no longer match — what it says about the place is history, and it
// is matched by place so that "lapsed" can be said at all.
const coversHere = "(de.live_key IS NULL OR (" + keyMatches + "))"

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
// **Read from the decisions outward, not from the findings inward.** What is
// joined is the set of open finding rows that have a decision of ours in this
// product, with what kind — built once from the decision table, which holds
// hundreds of rows where finding holds hundreds of thousands, and joined to
// the grouping by the finding's own identifier. The first version asked the
// question the other way round, as a correlated lookup per finding row, and
// that ran once for every open row in the build to say which groups had
// nothing: 241,479 probes to answer "undecided" on a build with no decisions
// at all. The counts are the same either way; a place with two decisions is
// one place, which is what folding to one row per finding keeps true.
//
// A decision belongs to a product and the two keys linking one to a finding do
// not: an issue is one row per identifier for the whole deployment, and a
// place identity is a hash of a consumer and a component with no product in it
// (see PlaceIdentity — deliberately, so a place is recognized across
// variants). Joined on those two alone this would match decisions in *every*
// product, which reports somebody else's triage as this product's state and
// tells a reader that a claim is pending in a product they cannot see; the
// product is a condition on the decision for that reason.
func (f Filter) byState(q *bun.SelectQuery) *bun.SelectQuery {
	state := strings.TrimSpace(f.State)
	outcome := strings.TrimSpace(f.Outcome)
	if state == "" && outcome == "" {
		return q
	}
	// The words are spelled here rather than taken from the triage package,
	// which imports this one — naming them there and reading them here is a
	// cycle. They are the values stored in the column either way, and the
	// enum on the endpoint is what keeps a caller from inventing a fifth.
	// Bound rather than spelled into the statement. These are compile-time
	// constants and nothing a caller supplies, so there is nothing to inject —
	// but a value in a placeholder is the rule (SEC-01), and a literal here is
	// the shape somebody copies to a place where it does matter.
	const (
		proposed = "proposed"
		approved = "approved"
		lapsed   = "lapsed"
	)
	// One row per open finding that has a decision of ours in this product,
	// saying which kinds. "Waiting" and "lapsed" count a claim in that state
	// whether or not it still stands; "approved" counts only the claim that
	// currently stands, because without that a judgment withdrawn eighteen
	// months ago still answers for its place.
	//
	// The finding's component and consumer are joined for the versions: a
	// live claim is about the place at the versions it was keyed on, and
	// matching it by place alone reports a claim made about one build's
	// version as standing over a second build shipping another.
	decided := q.NewSelect().
		TableExpr(`"decision" AS de`).
		Join("JOIN finding AS f2 ON f2.vulnerability_id = de.vulnerability_id"+
			" AND f2.place_identity = de.place_identity").
		Join("JOIN component AS c ON c.id = f2.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f2.consumer_id").
		ColumnExpr("f2.id AS finding_id").
		ColumnExpr("MAX(CASE WHEN de.state = ? THEN 1 ELSE 0 END) AS waiting", proposed).
		ColumnExpr("MAX(CASE WHEN de.state = ? AND de.live_key IS NOT NULL THEN 1 ELSE 0 END) AS approved",
			approved).
		ColumnExpr("MAX(CASE WHEN de.state = ? THEN 1 ELSE 0 END) AS lapsed", lapsed).
		// Which kind of judgment stands here, counted only for the claim that
		// currently stands: a dismissal withdrawn eighteen months ago must not
		// answer for its place, which is the same rule "approved" above holds.
		ColumnExpr("MAX(CASE WHEN de.live_key IS NOT NULL AND de.outcome = ? THEN 1 ELSE 0 END) AS this_outcome",
			outcome).
		Where("de.product_id = ?", f.ProductID).
		Where("f2.closed_at IS NULL").
		Where(coversHere).
		GroupExpr("f2.id")
	q = q.Join("LEFT JOIN (?) AS dd ON dd.finding_id = f.id", decided)

	if outcome != "" {
		// Every place answered the same way, not merely one of them: a group
		// where one place is dismissed and the rest are open is not a
		// dismissal, and listing it under "dismissed" is how a number stops
		// being one somebody can act on.
		q = q.Having("SUM(COALESCE(dd.this_outcome, 0)) = COUNT(*)")
	}
	switch state {
	case "agreed":
		return q.Having("SUM(COALESCE(dd.approved, 0)) = COUNT(*)")
	case "waiting":
		return q.Having("SUM(COALESCE(dd.waiting, 0)) > 0")
	case "lapsed":
		// Lapsed means nothing replaced it: a claim made again at the place
		// after the old one lapsed is waiting, which is what the row says,
		// and the filter has to find the row by the word it reads.
		return q.Having("SUM(COALESCE(dd.lapsed, 0)) > 0 AND SUM(COALESCE(dd.approved, 0)) = 0" +
			" AND SUM(COALESCE(dd.waiting, 0)) = 0")
	case "undecided":
		return q.Having("COUNT(dd.finding_id) = 0")
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

// inScope authorizes a selection and resolves it to the builds it covers.
//
// The three things every list over findings needs before it can read one:
// which product it is narrowing inside, what the reader may see there, and
// which builds the selection holds. Done once because two lists resolving them
// slightly differently would answer slightly different questions side by side.
func (s *Store) inScope(ctx context.Context, subject access.Subject, scope Scope,
	filter *Filter) (int64, []access.Visibility, []int64, error) {

	if scope.ProductID == nil {
		// Every list here correlates a decision by product and a place
		// identity carries none, so a selection without one would reach
		// decisions made in every product in the deployment.
		return 0, nil, nil, fmt.Errorf("read findings: the selection names no product")
	}
	productID := *scope.ProductID
	if !subject.Sees(productID) {
		return 0, nil, nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := access.Visible(subject, productID)
	if len(visible) == 0 {
		return 0, nil, nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	targets, err := s.Builds(ctx, scope)
	if err != nil {
		return 0, nil, nil, err
	}
	// Set here rather than trusted from the caller, for the reason above.
	filter.ProductID = productID
	// Who "mine" means, from the subject rather than from the request.
	filter.HeldBy = subject.ID
	// A subtree is a walk over one build's edges, so it is answerable only
	// where the selection is one build. Asked across several it is refused
	// rather than answered from whichever build sorted first — which is what
	// leaving the identifier at zero would have done, silently and emptily.
	switch {
	case len(targets) == 1:
		filter.TargetID = targets[0]
	case filter.Beneath != nil:
		return 0, nil, nil, fmt.Errorf("read findings beneath a component: a subtree is a walk"+
			" over one build's edges, and %d builds are in scope", len(targets))
	}
	return productID, visible, targets, nil
}

// Groups returns what is open in a selection, as the things somebody decides
// about rather than as one row per place.
//
// The selection is not always one build (UIX-53). With the branch or the
// variant left at "all" the page answers for every build under the product: a
// row is one issue at one component still, and its places are counted across
// every build the group is in. What a row cannot carry there is the way down,
// because a chain belongs to one build's graph — so those columns are filled
// only where the selection is a single build, and across several the row names
// one of them and says how many hold it instead. Naming one as though it were
// the answer is the thing being avoided.
func (s *Store) Groups(ctx context.Context, subject access.Subject, scope Scope, limit, offset int,
	filter Filter) ([]Group, int, error) {
	productID, visible, targets, err := s.inScope(ctx, subject, scope, &filter)
	if err != nil {
		return nil, 0, err
	}
	if len(targets) == 0 {
		return nil, 0, nil
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// The page in two statements: which groups, then what is known about
	// them.
	//
	// The first groups every open finding in the build by issue and component
	// and keeps the fifty most urgent. It reads four columns, all of them in
	// finding's covering index, so however large the build is this is one
	// walk of one index with nothing looked up. Everything else the row shows
	// — likelihood and score, how far it has been decided, how many ways
	// down there are, the fix — is read in a second statement over the fifty
	// groups on the page. The first version asked all of it in one statement,
	// and the five correlated decision lookups per row ran against every one
	// of a build's 240,945 open rows to produce fifty answers.
	heads, total, err := s.heads(ctx, targets, visible, limit, offset, filter)
	if err != nil {
		return nil, 0, err
	}

	known, err := s.decorate(ctx, targets, productID, visible, heads, filter)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]decorated, 0, len(heads))
	for _, head := range heads {
		row := known[groupKey{head.VulnerabilityID, head.ComponentID}]
		row.groupHead = head
		rows = append(rows, row)
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
	oneBuild := len(targets) == 1
	var chains map[int64][]graph.Step
	var where map[int64]build
	if oneBuild {
		wanted := make([]int64, 0, len(rows))
		for _, row := range rows {
			if row.ConsumerID != nil {
				wanted = append(wanted, *row.ConsumerID)
			} else {
				wanted = append(wanted, row.ComponentID)
			}
		}
		if chains, err = graph.NewStore(s.db).Chains(ctx, subject, targets[0], wanted); err != nil {
			return nil, 0, err
		}
	} else {
		// Across builds a row names one of them, so that a screen has
		// somewhere to link to and an action has a build to name. Read for the
		// page's builds in one statement rather than per row.
		at := make([]int64, 0, len(rows))
		for _, row := range rows {
			at = append(at, row.TargetID)
		}
		if where, err = buildsNamed(ctx, s.db, at); err != nil {
			return nil, 0, err
		}
	}

	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		group := Group{
			Places: row.Places, Answered: row.Answered,
			Urgency: row.Urgency, Exploited: Rank(row.Urgency).Exploited(),
			LikelihoodPPM: row.LikelihoodPPM, ScoreCenti: row.ScoreCenti,
			FixState: FixState(row.FixState), FixedIn: row.FixedIn,
			Matched:  Matched(row.Matched),
			State:    stateWord(row.Places, row.AnyClaim, row.Waiting, row.Approved, row.Lapsed),
			SentBack: row.SentBack > 0,
		}
		if issue, held := named[row.VulnerabilityID]; held {
			group.Vulnerability, group.Severity = issue.Identifier, issue.Severity
		}
		if component, held := shipped[row.ComponentID]; held {
			group.Component, group.Version = component.Name, component.Version
			group.Ecosystem = graph.EcosystemOf(component.Purl)
			if component.UpstreamVersion != "" {
				group.Upstream = component.UpstreamName + " " + component.UpstreamVersion
			}
		}
		if oneBuild {
			// How many distinct ways down there are: the consumers this
			// component has here, plus one for the build pulling it in
			// directly.
			group.Chains = row.Consumers
			if row.Direct > 0 {
				group.Chains++
			}
			down := chains[row.ComponentID]
			if row.ConsumerID != nil {
				down = chains[*row.ConsumerID]
			}
			group.Owner, group.Parent, group.Middle = Ends(down)
		} else if at, held := where[row.TargetID]; held {
			group.Builds = row.Builds
			// The way down is left empty rather than filled from this one
			// build: the group sits in every build the count names, and each
			// of them reaches it its own way.
			group.Stream, group.Variant = at.Stream, at.Variant
		}
		groups = append(groups, group)
	}
	return groups, total, nil
}

// build names one of the places a group sits, for a row that spans several.
type build struct {
	ID      int64  `bun:"id"`
	Stream  string `bun:"stream"`
	Variant string `bun:"variant"`
}

// buildsNamed names the builds a page's rows point at.
//
// One statement for the page rather than one per row, and the names are the
// ones somebody declared: a path resolves a name without regard to capitals
// (MDL-21), so what was typed is both what reads correctly and what routes.
func buildsNamed(ctx context.Context, db bun.IDB, ids []int64) (map[int64]build, error) {
	named := make(map[int64]build, len(ids))
	if len(ids) == 0 {
		return named, nil
	}
	var rows []build
	if err := db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		ColumnExpr("tg.id AS id").
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		Where("tg.id IN (?)", bun.List(ids)).
		Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("name the builds a page sits in: %w", err)
	}
	for _, row := range rows {
		named[row.ID] = row
	}
	return named, nil
}

// groupHead is one group of the findings list as the covering index knows it:
// which issue in which component, at how many places, and the worst urgency
// among them. Everything the list is filtered, ordered and paged by.
type groupHead struct {
	VulnerabilityID int64 `bun:"vulnerability_id"`
	ComponentID     int64 `bun:"component_id"`
	Places          int   `bun:"places"`
	Urgency         int64 `bun:"urgency"`
	// Total is how many groups the filter admits, the same on every row.
	Total int `bun:"total"`
}

// groupKey names one group of the list.
type groupKey struct{ vulnerabilityID, componentID int64 }

// decorated is a group of the list with what the page shows about it read in.
type decorated struct {
	groupHead
	Answered      int    `bun:"answered"`
	LikelihoodPPM int    `bun:"likelihood_ppm"`
	ScoreCenti    int    `bun:"score_centi"`
	FixState      string `bun:"fix_state"`
	FixedIn       string `bun:"fixed_in"`
	Matched       string `bun:"matched"`
	ConsumerID    *int64 `bun:"consumer_id"`
	Consumers     int    `bun:"consumers"`
	Direct        int    `bun:"direct"`
	Builds        int    `bun:"builds"`
	TargetID      int64  `bun:"target_id"`
	AnyClaim      int    `bun:"any_claim"`
	Waiting       int    `bun:"waiting_here"`
	Approved      int    `bun:"approved_here"`
	Lapsed        int    `bun:"lapsed_here"`
	SentBack      int    `bun:"sent_back_here"`
}

// heads reads one page of the findings list's groups, most urgent first, from
// finding's covering index and nothing else, and how many groups there are.
//
// The total rides on the page. It is counted through the same filter as the
// page — a total that ignores the narrowing is worse than no total: it
// reports how much there is to decide about, which is the figure people
// quote, while the list beside it shows something else — and it used to be
// a second statement making the same grouping over the same rows to count
// what the first had just grouped. `COUNT(*) OVER ()` is the number of rows
// the grouping produced after the HAVING clauses and before the limit,
// which is exactly that, on all four engines (window functions are in each
// of them), for the cost of nothing. Where the page comes back empty — an
// offset past the end — there is no row to carry it and it is counted
// separately.
func (s *Store) heads(ctx context.Context, targets []int64, visible []access.Visibility,
	limit, offset int, filter Filter) ([]groupHead, int, error) {

	var heads []groupHead
	page := s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("COUNT(*) AS places").
		// The most urgent place this issue sits at. A group is one decision
		// about one issue in one component, so what should decide where that
		// decision appears is the worst of what it covers.
		ColumnExpr("MAX(f.urgency) AS urgency").
		ColumnExpr("COUNT(*) OVER () AS total").
		Where("f.target_id IN (?)", bun.List(targets)).
		Where("f.closed_at IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id").
		// Ordered by urgency rather than by how widespread something is.
		// Sorting by place count puts whatever ships in the most places at the
		// top, which on a real image is the kernel — everywhere, and not
		// therefore the thing to look at first. What somebody with an hour
		// needs at the top is what is being exploited.
		OrderExpr("urgency DESC, places DESC, f.vulnerability_id, f.component_id").
		Limit(limit).Offset(offset)
	if err := filter.narrow(page).Scan(ctx, &heads); err != nil {
		return nil, 0, fmt.Errorf("read what is open: %w", err)
	}
	if len(heads) > 0 {
		return heads, heads[0].Total, nil
	}
	counted := s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.vulnerability_id").
		Where("f.target_id IN (?)", bun.List(targets)).
		Where("f.closed_at IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id")
	total, err := s.db.NewSelect().
		TableExpr("(?) AS grouped", filter.narrow(counted)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is open: %w", err)
	}
	return heads, total, nil
}

// decorate reads what the page shows about each of its groups, in one
// statement over the groups named and no other.
//
// Narrowed through the same filter as the groups were chosen by, so every
// number here is over exactly the places the group was counted over: a list
// narrowed to what sits inside one container reports how many of *those*
// places are answered, not how many places there are anywhere.
func (s *Store) decorate(ctx context.Context, targets []int64, productID int64,
	visible []access.Visibility, heads []groupHead, filter Filter) (map[groupKey]decorated, error) {

	known := make(map[groupKey]decorated, len(heads))
	if len(heads) == 0 {
		return known, nil
	}
	issues := make([]int64, 0, len(heads))
	components := make([]int64, 0, len(heads))
	for _, head := range heads {
		issues = append(issues, head.VulnerabilityID)
		components = append(components, head.ComponentID)
	}

	// How far each group has been decided, counted the way the state filter
	// counts it, so the row and the filter cannot disagree. Four correlated
	// counts over our decisions in this product at each place and at the
	// versions the place holds, plus whether any live claim is with its
	// author.
	decided := func(alias, condition string) string {
		return `SUM(CASE WHEN EXISTS (SELECT 1 FROM "decision" AS de
			WHERE de.product_id = ?
			  AND de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity
			  AND ` + coversHere + condition + `) THEN 1 ELSE 0 END) AS ` + alias
	}
	var rows []decorated
	q := s.db.NewSelect().
		TableExpr("finding AS f").
		// Joined for the likelihood and the score. Likelihood ranks above
		// severity, so a list that orders by it and does not show it looks
		// unsorted.
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		// And for the versions a decision is keyed on. Two primary-key
		// lookups per place on the page, so the correlated counts can ask
		// whether a claim is about the versions shipping here rather than
		// about the place at whatever versions it once held.
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("MAX(COALESCE(v.likelihood_ppm, 0)) AS likelihood_ppm").
		ColumnExpr("MAX(COALESCE(v.score_centi, 0)) AS score_centi").
		ColumnExpr("SUM(CASE WHEN f.suppressed_by IS NULL THEN 0 ELSE 1 END) AS answered").
		ColumnExpr("MIN(f.fix_state) AS fix_state").
		ColumnExpr("MIN(f.fixed_in) AS fixed_in").
		// Any of them: a group is an issue at a component, every place of it
		// comes from one line of a scanner's report, and the applier writes
		// that line to all of them.
		ColumnExpr("MIN(COALESCE(f.matched, '')) AS matched").
		// One of the ways down, and how many there are. MIN passes over the
		// places the build pulls in directly, whose consumer is null, so those
		// are counted separately rather than being read as "no route at all".
		ColumnExpr("MIN(f.consumer_id) AS consumer_id").
		ColumnExpr("COUNT(DISTINCT f.consumer_id) AS consumers").
		ColumnExpr("SUM(CASE WHEN f.consumer_id IS NULL THEN 1 ELSE 0 END) AS direct").
		// How many builds in the selection hold this group, and one of them to
		// name. Both are one where the selection is a single build, which is
		// why the row says nothing about either there.
		ColumnExpr("COUNT(DISTINCT f.target_id) AS builds").
		ColumnExpr("MIN(f.target_id) AS target_id").
		ColumnExpr(decided("any_claim", ""), productID).
		ColumnExpr(decided("waiting_here", " AND de.state = ? AND de.live_key IS NOT NULL"),
			productID, "proposed").
		ColumnExpr(decided("approved_here", " AND de.state = ? AND de.live_key IS NOT NULL"),
			productID, "approved").
		ColumnExpr(decided("lapsed_here", " AND de.state = ?"), productID, "lapsed").
		ColumnExpr(decided("sent_back_here",
			" AND de.state = ? AND de.live_key IS NOT NULL AND de.sent_back_at IS NOT NULL"),
			productID, "proposed").
		Where("f.target_id IN (?)", bun.List(targets)).
		Where("f.closed_at IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		// The page's issues and the page's components, as two lists. That
		// admits an issue from one row in the component of another, and
		// those groups are read and dropped; what it buys is the index. As
		// fifty `(issue = ? AND component = ?)` pairs joined by OR, SQLite
		// walked every open row in the build against the fifty — 150 ms,
		// half the page — where two lists are fifty index seeks, 2 ms. A
		// page is mostly one component's issues, so the extra groups are
		// few, and each is one seek too.
		Where("f.vulnerability_id IN (?)", bun.List(issues)).
		Where("f.component_id IN (?)", bun.List(components)).
		GroupExpr("f.vulnerability_id, f.component_id")
	if err := filter.narrow(q).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read about what is open: %w", err)
	}
	for _, row := range rows {
		known[groupKey{row.VulnerabilityID, row.ComponentID}] = row
	}
	return known, nil
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
		Column("id", "purl", "name", "version", "upstream_name", "upstream_version").
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
	// Links are worked out from the names held here rather than supplied by a
	// scanner, which is why they are kept apart from References. What a report
	// points at is whatever its data carried; these resolve because an
	// identifier names a record and a package identifier names a package.
	Links []Link

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
	// Recorded says a person entered this rather than a scanner reporting it,
	// which is what decides whether a person may close it (REM-28). Everything
	// else about it behaves the same, so this is the one place the difference
	// has to be visible.
	Recorded bool

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
	// AssignedTo is who is dealing with this, by sign-in identity, or empty
	// where nobody is. One name for the whole finding, because assignment is
	// set for a group at once; where the places somehow disagree it is empty
	// rather than naming one of them.
	AssignedTo string
	// Matched says how the scanner reached this, and MatchedFrom where that
	// match came from. One answer for every place, because they all come from
	// one line of a scanner's report.
	//
	// The source is here rather than on the issue because one issue reached
	// through two ecosystems has two answers and the issue can hold one. An
	// Alpine package linking to Debian's tracker is what the other way looks
	// like.
	Matched     Matched
	MatchedFrom string
	// MatchedIn and MatchedRange are the evidence for the judgment Matched
	// asks for: which body of data answered, and the version range the match
	// fired on. A range naming no packaging revision, read beside a version
	// that has one, is the whole argument in a line.
	MatchedIn    string
	MatchedRange string
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
	// Claim is the action that decision was one row of. A claim covering
	// several places can then name them rather than only count them: "one
	// location" says how big a judgment was and not which code it was about,
	// and on a finding that is only partly decided that is the whole question.
	Claim *int64
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
	visible := access.Visible(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []struct {
		PlaceIdentity string     `bun:"place_identity"`
		Consumer      string     `bun:"consumer"`
		ConsumerID    *int64     `bun:"consumer_id"`
		Decision      *int64     `bun:"decision"`
		Claim         *int64     `bun:"claim"`
		Suppressed    bool       `bun:"suppressed"`
		Urgency       int64      `bun:"urgency"`
		FixState      string     `bun:"fix_state"`
		FixedIn       string     `bun:"fixed_in"`
		FixedAt       *time.Time `bun:"fixed_at"`
		Matched       string     `bun:"matched"`
		Kind          string     `bun:"kind"`
		MatchedFrom   string     `bun:"matched_from"`
		MatchedIn     string     `bun:"matched_in"`
		MatchedRange  string     `bun:"matched_range"`
		ArrivedFrom   string     `bun:"arrived_from"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.place_identity AS place_identity").
		ColumnExpr("COALESCE(uc.name, '') AS consumer").
		ColumnExpr("f.consumer_id AS consumer_id").
		// A live claim standing at this place, whatever its state. Proposed
		// and waiting counts: it is answered as far as the person looking at
		// it is concerned, and showing it as untouched invites a second claim
		// about the same code.
		//
		// This product's, at the versions shipping here. A place identity
		// carries no product, so without the first a claim in another product
		// that ships the same component reads as standing on this screen; and
		// a live claim is about the versions it was keyed on, so without the
		// second a claim about last release's version answers for this one.
		ColumnExpr(`(SELECT MIN(de.id) FROM "decision" AS de
			WHERE de.product_id = ?
			  AND de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity
			  AND de.live_key IS NOT NULL
			  AND `+keyMatches+`) AS decision`, productID).
		// And which claim that decision is a row of, so that a claim shown on
		// the finding can name the places it covers rather than only count
		// them. At most one live decision stands per combination of code
		// (TRI-33), so this is the same single row the column above reaches.
		ColumnExpr(`(SELECT MIN(de.claim_id) FROM "decision" AS de
			WHERE de.product_id = ?
			  AND de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity
			  AND de.live_key IS NOT NULL
			  AND `+keyMatches+`) AS claim`, productID).
		ColumnExpr("CASE WHEN f.suppressed_by IS NULL THEN ? ELSE ? END AS suppressed", false, true).
		ColumnExpr("f.urgency AS urgency").
		ColumnExpr("f.fix_state AS fix_state").
		ColumnExpr("f.fixed_in AS fixed_in").
		ColumnExpr("f.fixed_at AS fixed_at").
		ColumnExpr("COALESCE(f.matched, '') AS matched").
		ColumnExpr("COALESCE(f.matched_from, '') AS matched_from").
		ColumnExpr("COALESCE(f.matched_in, '') AS matched_in").
		ColumnExpr("COALESCE(f.matched_range, '') AS matched_range").
		ColumnExpr("COALESCE(f.arrived_from, '') AS arrived_from").
		ColumnExpr("f.kind AS kind").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.component_id = ?", componentID).
		Where("f.closed_at IS NULL").
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
	// Any place answers. They all come from one line of a scanner's report,
	// which the applier writes to every place of the group.
	evidence.Matched = Matched(rows[0].Matched)
	evidence.Recorded = Kind(rows[0].Kind) == Entered
	evidence.MatchedFrom = rows[0].MatchedFrom
	evidence.MatchedIn = rows[0].MatchedIn
	evidence.MatchedRange = rows[0].MatchedRange
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
	// Worked out here rather than stored: an address derived from two names
	// cannot go stale while the names are right, and storing it would be a
	// second copy of the templates to keep in step.
	evidence.Links = links(issue.Identifier, evidence.Aliases, component.Purl)
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
			Suppressed: row.Suppressed, Decision: row.Decision, Claim: row.Claim,
			Urgency: row.Urgency,
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

	// Who is dealing with it. Read here rather than left to a caller, so that
	// the screen somebody reads a finding on is the screen they can hand it
	// over from — being able to record a judgment about something and not to
	// say who is dealing with it is a strange half of the same job.
	//
	// One name for the whole finding, and empty where the places disagree: the
	// assignment is set for a group at once, so a disagreement is a state
	// nobody chose and naming one of them would hide it.
	held, err := s.heldBy(ctx, targetID, vulnerabilityID, componentID)
	if err != nil {
		return nil, err
	}
	evidence.AssignedTo = held
	return evidence, nil
}

// heldBy is the sign-in identity of whoever is dealing with a finding, or
// empty where nobody is or the places do not agree.
func (s *Store) heldBy(ctx context.Context, targetID, vulnerabilityID, componentID int64) (string, error) {
	var holders []struct {
		Identity *string `bun:"identity"`
	}
	err := s.db.NewSelect().
		TableExpr("finding AS f").
		Join(`LEFT JOIN "person" AS p ON p.id = f.assigned_to`).
		ColumnExpr("p.identity AS identity").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.component_id = ?", componentID).
		Where("f.closed_at IS NULL").
		GroupExpr("p.identity").
		Scan(ctx, &holders)
	if err != nil {
		return "", fmt.Errorf("read who is dealing with this: %w", err)
	}
	if len(holders) != 1 || holders[0].Identity == nil {
		return "", nil
	}
	return *holders[0].Identity, nil
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
	visible := access.Visible(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	narrow := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = q.TableExpr("finding AS f").
			Where("f.target_id = ?", targetID).
			Where("f.component_id = ?", componentID).
			Where("f.closed_at IS NULL").
			Where("f.visibility IN (?)", bun.List(visible))
		if contains != "" {
			// Matched against what a report says, which is all that is held
			// about where a flaw lives. Nothing here knows a kernel from a
			// font library. Asked as a membership test rather than a join so
			// the grouping stays on finding's covering index.
			q = q.Where("f.vulnerability_id IN (?)",
				q.NewSelect().TableExpr("vulnerability AS v").Column("v.id").
					Where("LOWER(v.description) LIKE ?", "%"+strings.ToLower(contains)+"%"))
		}
		return q
	}

	// The page in two statements, as the findings list is read: which issues,
	// off the covering index, and then what is shown about each. On a real
	// image one component holds most of the build's open rows — the kernel,
	// 222,435 of 272,539 — and grouping them with three joins under the
	// aggregate cost 0.35 s where the index alone answers in 0.04 s. The
	// total rides on the page, as the findings list's does.
	var heads []struct {
		VulnerabilityID int64 `bun:"vulnerability_id"`
		Places          int   `bun:"places"`
		Total           int   `bun:"total"`
	}
	err = narrow(s.db.NewSelect()).
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("COUNT(*) AS places").
		ColumnExpr("COUNT(*) OVER () AS total").
		GroupExpr("f.vulnerability_id").
		OrderExpr("MAX(f.urgency) DESC, f.vulnerability_id").
		Limit(limit).Offset(offset).
		Scan(ctx, &heads)
	if err != nil {
		return nil, 0, fmt.Errorf("read what is open against this component: %w", err)
	}
	total := 0
	if len(heads) > 0 {
		total = heads[0].Total
	} else {
		// Grouped and counted, not COUNT DISTINCT: with no GROUP BY, Count()
		// emits its own count(*) and the expression never reaches the
		// statement, so the total counted places while the list counts
		// issues.
		if total, err = s.db.NewSelect().
			TableExpr(`(?) AS "grouped"`, narrow(s.db.NewSelect()).
				ColumnExpr("f.vulnerability_id").GroupExpr("f.vulnerability_id")).
			Count(ctx); err != nil {
			return nil, 0, fmt.Errorf("count what is open against this component: %w", err)
		}
	}
	issues := make([]int64, 0, len(heads))
	for _, head := range heads {
		issues = append(issues, head.VulnerabilityID)
	}

	type shown struct {
		VulnerabilityID int64  `bun:"vulnerability_id"`
		Severity        int    `bun:"severity_centi"`
		FixedIn         string `bun:"fixed_in"`
	}
	about := map[int64]shown{}
	if len(issues) > 0 {
		var rows []shown
		err = s.db.NewSelect().
			TableExpr("finding AS f").
			Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
			ColumnExpr("f.vulnerability_id AS vulnerability_id").
			ColumnExpr("MIN(COALESCE(v.score_centi, 0)) AS severity_centi").
			ColumnExpr("MIN(COALESCE(f.fixed_in, '')) AS fixed_in").
			Where("f.target_id = ?", targetID).
			Where("f.component_id = ?", componentID).
			Where("f.closed_at IS NULL").
			Where("f.visibility IN (?)", bun.List(visible)).
			Where("f.vulnerability_id IN (?)", bun.List(issues)).
			GroupExpr("f.vulnerability_id").
			Scan(ctx, &rows)
		if err != nil {
			return nil, 0, fmt.Errorf("read about what is open against this component: %w", err)
		}
		for _, row := range rows {
			about[row.VulnerabilityID] = row
		}
	}

	// Every place, fetched for the page of issues rather than one arbitrary
	// place per issue. A decision is keyed on a place, so a claim built from
	// MIN(place_identity) covers one consumer and leaves the rest open while
	// reporting that it covered them.
	everywhere, err := s.placesOf(ctx, targetID, componentID, issues, visible)
	if err != nil {
		return nil, 0, err
	}

	at := make([]Deciding, 0, len(heads))
	for _, head := range heads {
		row := about[head.VulnerabilityID]
		for _, place := range everywhere[head.VulnerabilityID] {
			place.ProductID = productID
			place.VulnerabilityID = head.VulnerabilityID
			place.SeverityCenti = row.Severity
			place.FixedIn = row.FixedIn
			place.Places = head.Places
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
		Where("f.closed_at IS NULL").
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
	// Ecosystem is the kind of package, carried for the same reason a
	// finding's row carries it: a name at a version does not tell two rows
	// apart on its own.
	Ecosystem string
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
func (s *Store) ComponentGroups(ctx context.Context, subject access.Subject, scope Scope,
	limit, offset int, filter Filter) ([]ComponentGroup, int, error) {

	_, visible, targets, err := s.inScope(ctx, subject, scope, &filter)
	if err != nil {
		return nil, 0, err
	}
	if len(targets) == 0 {
		return nil, 0, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var rows []struct {
		ComponentID int64 `bun:"component_id"`
		Issues      int   `bun:"issues"`
		Places      int   `bun:"places"`
		Urgency     int64 `bun:"urgency"`
		Total       int   `bun:"total"`
	}
	// Everything read here is in finding's covering index, so this is one
	// walk of it however large the build is; whether anything is exploited
	// is read off the urgency, which ranks it in a band of its own.
	page := s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.component_id AS component_id").
		// Distinct issues rather than rows, because that is what the findings
		// list shows and therefore what hiding this component would remove
		// from it.
		ColumnExpr("COUNT(DISTINCT f.vulnerability_id) AS issues").
		ColumnExpr("COUNT(*) AS places").
		ColumnExpr("MAX(f.urgency) AS urgency").
		// The total rides on the page, as the findings list's does.
		ColumnExpr("COUNT(*) OVER () AS total").
		Where("f.target_id IN (?)", bun.List(targets)).
		Where("f.closed_at IS NULL").
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

	total := 0
	if len(rows) > 0 {
		total = rows[0].Total
	} else {
		counted := s.db.NewSelect().
			TableExpr("finding AS f").
			ColumnExpr("f.component_id").
			Where("f.target_id IN (?)", bun.List(targets)).
			Where("f.closed_at IS NULL").
			Where("f.visibility IN (?)", bun.List(visible)).
			GroupExpr("f.component_id")
		if total, err = s.db.NewSelect().
			TableExpr("(?) AS grouped", filter.narrow(counted)).
			Count(ctx); err != nil {
			return nil, 0, fmt.Errorf("count what is open by component: %w", err)
		}
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
			Exploited: Rank(row.Urgency).Exploited(), Urgency: row.Urgency,
		}
		if named, ok := shipped[row.ComponentID]; ok {
			group.Component = named.Name
			group.Version = named.Version
			group.Upstream = named.UpstreamVersion
			group.Ecosystem = graph.EcosystemOf(named.Purl)
		}
		groups = append(groups, group)
	}
	return groups, total, nil
}
