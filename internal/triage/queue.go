package triage

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

// Waiting is one claim somebody has to look at, with what an approver needs in
// order to judge it.
//
// Everything here is carried rather than left to be fetched per row. A
// reviewer works down a list, and a list where judging each row means opening
// it is a list that gets approved without being read — which is the failure
// the queue exists to prevent, arriving by a different route.
//
// One of these is one claim — one proposer's action — however many decisions
// it wrote (TRI-45). Decision is a representative row: the earliest, which is
// the same on every engine and every run.
type Waiting struct {
	Claim    Claim
	Decision Decision
	// Reasoning is what the proposer wrote, as it currently stands on the
	// representative row.
	Reasoning string
	// PreviouslyApproved says this was agreed to before and came back — either
	// because the reasoning was revised under the approval or because the code
	// moved. Somebody meeting it again should know they are re-reading rather
	// than seeing it for the first time.
	PreviouslyApproved bool
	// DeferredSoFar is the total time this finding has been put off, across
	// every deferral. What decides whether a deferral needs agreement is the
	// cumulative time, not the length of the one being asked about.
	DeferredSoFar time.Duration
	// Decisions, Issues and Places are how big the claim is: rows written,
	// distinct issues, distinct places.
	Decisions, Issues, Places int
	// Builds names every build the claim's rows currently cover, as the
	// stream and variant display names joined with a middle dot.
	Builds []string
	// Outliers is what in a bulk set does not look like the rest. Only for a
	// claim over many issues; nil otherwise.
	Outliers *Outliers
}

// Outliers is what an approver of a bulk claim checks instead of reading
// every row: the handful that contradict the shape of the claim (TRI-46).
type Outliers struct {
	// Exploited, Severe, Fixable and Unmatched count the distinct issues in
	// the claim with that property. Severe is critical or high; Unmatched is
	// an issue whose description does not contain the term the set was
	// narrowed by, where that term is known.
	Exploited, Severe, Fixable, Unmatched int
	// Rows are the issues that stood out, exploited first and then by
	// severity, capped.
	Rows []Outlier
}

// Outlier is one issue in a bulk claim that does not look like the rest.
type Outlier struct {
	DecisionID    int64
	Vulnerability string
	Severity      string
	Exploited     bool
	FixedIn       string
	Description   string
	// Why says which of the four things made it stand out.
	Why []string
}

// outlierRows is how many outliers a queue card carries. Enough to read;
// the counts say how many there are.
const outlierRows = 20

// Queue returns what is waiting for somebody, newest first, one entry per
// claim.
//
// Narrowed to what the asker may act on, in the query. A reviewer who cannot
// triage a product should not be shown its claims at all — a queue is a work
// list, and one containing work somebody cannot do teaches them to skip rows.
//
// A claim is shown only where the reader may act on every row in it. Acting on
// a claim is acting on the argument, which does not come in halves: shown the
// part they may approve, a reader would agree to words whose other half stays
// waiting on somebody else, and the count beside the card would be wrong.
func (s *Store) Queue(ctx context.Context, subject access.Subject, limit, offset int) ([]Waiting, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// The claims with a waiting row this person may act on, ordered by the
	// newest row in each. Grouped in the statement rather than here, so a
	// page is a page of claims and the count counts claims.
	waitingClaims := func() *bun.SelectQuery {
		q := s.db.NewSelect().Model((*Decision)(nil)).
			ColumnExpr("de.claim_id AS claim_id").
			ColumnExpr("MAX(de.id) AS newest").
			GroupExpr("de.claim_id")
		q = approvableBy(waiting(q, s.now()), subject, "de")
		return q.Where("NOT EXISTS (?)", notApprovableBy(
			s.db.NewSelect().TableExpr(`"decision" AS "other"`).ColumnExpr("1").
				Where(`"other".claim_id = de.claim_id`), subject, `"other"`))
	}

	total, err := s.db.NewSelect().
		TableExpr("(?) AS \"waiting_claims\"", waitingClaims()).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is waiting: %w", err)
	}

	var page []struct {
		ClaimID int64 `bun:"claim_id"`
		Newest  int64 `bun:"newest"`
	}
	if err := waitingClaims().OrderExpr("newest DESC").
		Limit(limit).Offset(offset).Scan(ctx, &page); err != nil {
		return nil, 0, fmt.Errorf("read what is waiting: %w", err)
	}
	if len(page) == 0 {
		return nil, total, nil
	}
	ids := make([]int64, 0, len(page))
	for _, row := range page {
		ids = append(ids, row.ClaimID)
	}

	var claims []Claim
	if err := s.db.NewSelect().Model(&claims).
		Where("id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("read what is waiting: %w", err)
	}
	byID := make(map[int64]Claim, len(claims))
	for _, claim := range claims {
		byID[claim.ID] = claim
	}

	// Every row of every claim on the page, in one read. The representative
	// is the earliest row; the sizes are counted over all of them.
	var rows []Decision
	if err := s.db.NewSelect().Model(&rows).
		Where("claim_id IN (?)", bun.List(ids)).Order("id ASC").Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("read what is waiting: %w", err)
	}
	first := map[int64]Decision{}
	issues := map[int64]map[int64]bool{}
	places := map[int64]map[string]bool{}
	count := map[int64]int{}
	all := make([]int64, 0, len(rows))
	for _, row := range rows {
		if _, seen := first[row.ClaimID]; !seen {
			first[row.ClaimID] = row
			issues[row.ClaimID] = map[int64]bool{}
			places[row.ClaimID] = map[string]bool{}
		}
		issues[row.ClaimID][row.VulnerabilityID] = true
		places[row.ClaimID][row.PlaceIdentity] = true
		count[row.ClaimID]++
		all = append(all, row.ID)
	}

	representatives := make([]Decision, 0, len(page))
	for _, row := range page {
		representatives = append(representatives, first[row.ClaimID])
	}
	reasoning, err := s.currentReasoning(ctx, representatives)
	if err != nil {
		return nil, 0, err
	}
	seenBefore, err := s.everApproved(ctx, all)
	if err != nil {
		return nil, 0, err
	}
	builds, err := s.buildsCovered(ctx, subject, ids)
	if err != nil {
		return nil, 0, err
	}

	deferred, err := s.deferredSoFar(ctx, representatives)
	if err != nil {
		return nil, 0, err
	}
	var bulk []Claim
	for _, row := range page {
		if claim := byID[row.ClaimID]; claim.Kind == TogetherClaim {
			bulk = append(bulk, claim)
		}
	}
	outliers, err := s.outliersFor(ctx, subject, bulk)
	if err != nil {
		return nil, 0, err
	}

	out := make([]Waiting, 0, len(page))
	for _, row := range page {
		claim := byID[row.ClaimID]
		representative := first[row.ClaimID]
		before := false
		for _, each := range rows {
			if each.ClaimID == row.ClaimID && seenBefore[each.ID] {
				before = true
				break
			}
		}
		one := Waiting{
			Claim: claim, Decision: representative,
			Reasoning:          reasoning[representative.ID],
			PreviouslyApproved: before,
			DeferredSoFar:      deferred[representative.ID],
			Decisions:          count[row.ClaimID],
			Issues:             len(issues[row.ClaimID]),
			Places:             len(places[row.ClaimID]),
			Builds:             builds[row.ClaimID],
		}
		if claim.Kind == TogetherClaim {
			one.Outliers = outliers[claim.ID]
		}
		out = append(out, one)
	}
	return out, total, nil
}

// buildsCovered names the builds each claim's rows currently cover: every
// open finding at the row's place, at the versions the row was written
// against — the same match a finding makes when it asks whether a decision
// applies to it.
//
// Narrowed to the findings the reader may see. A build named here is a
// statement that the build holds the issue, and a decision somebody may read
// can match findings they may not.
func (s *Store) buildsCovered(ctx context.Context, subject access.Subject, claims []int64) (map[int64][]string, error) {
	var rows []struct {
		ClaimID int64  `bun:"claim_id"`
		Stream  string `bun:"stream"`
		Variant string `bun:"variant"`
	}
	query := s.db.NewSelect().
		TableExpr("decision AS de").
		// The decision on the outside of the join, for the reason Describe
		// gives: SQLite otherwise starts from every open finding.
		Join("CROSS JOIN finding AS f").
		Where("f.vulnerability_id = de.vulnerability_id AND f.place_identity = de.place_identity").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		ColumnExpr("de.claim_id AS claim_id").
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		Where("de.claim_id IN (?)", bun.List(claims)).
		Where("f.closed_run_id IS NULL").
		Where("st.product_id = de.product_id").
		Where("COALESCE(de.component_upstream_version, '') = " + finding.ComponentUpstreamExpr).
		Where("COALESCE(de.consumer_upstream_version, '') = " + finding.ConsumerUpstreamExpr)
	err := readableFindings(query, subject, "f", "st.product_id").
		GroupExpr("de.claim_id, st.display_name, va.display_name").
		OrderExpr("de.claim_id, st.display_name, va.display_name").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read which builds these cover: %w", err)
	}
	builds := map[int64][]string{}
	for _, row := range rows {
		builds[row.ClaimID] = append(builds[row.ClaimID], row.Stream+" \u00b7 "+row.Variant)
	}
	return builds, nil
}

// outliersFor reads the outliers of every bulk claim on a page, in three
// statements for the page rather than three per claim: which issues each
// claim covers, what those issues are, and where a fix is known.
func (s *Store) outliersFor(ctx context.Context, subject access.Subject, claims []Claim) (map[int64]*Outliers, error) {
	out := make(map[int64]*Outliers, len(claims))
	if len(claims) == 0 {
		return out, nil
	}
	claimIDs := make([]int64, 0, len(claims))
	for _, claim := range claims {
		claimIDs = append(claimIDs, claim.ID)
		out[claim.ID] = &Outliers{Rows: []Outlier{}}
	}

	var heads []struct {
		ClaimID         int64 `bun:"claim_id"`
		VulnerabilityID int64 `bun:"vulnerability_id"`
		DecisionID      int64 `bun:"decision_id"`
	}
	if err := s.db.NewSelect().
		TableExpr("decision AS de").
		ColumnExpr("de.claim_id AS claim_id").
		ColumnExpr("de.vulnerability_id AS vulnerability_id").
		ColumnExpr("MIN(de.id) AS decision_id").
		Where("de.claim_id IN (?)", bun.List(claimIDs)).
		GroupExpr("de.claim_id, de.vulnerability_id").
		Scan(ctx, &heads); err != nil {
		return nil, fmt.Errorf("read what a bulk claim covers: %w", err)
	}
	if len(heads) == 0 {
		return out, nil
	}
	issueIDs := make([]int64, 0, len(heads))
	covers := map[int64]map[int64]int64{}
	for _, head := range heads {
		issueIDs = append(issueIDs, head.VulnerabilityID)
		if covers[head.ClaimID] == nil {
			covers[head.ClaimID] = map[int64]int64{}
		}
		covers[head.ClaimID][head.VulnerabilityID] = head.DecisionID
	}

	var issues []finding.Vulnerability
	if err := s.db.NewSelect().Model(&issues).
		Where("id IN (?)", bun.List(issueIDs)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read the issues a bulk claim covers: %w", err)
	}
	byIssue := make(map[int64]finding.Vulnerability, len(issues))
	for _, issue := range issues {
		byIssue[issue.ID] = issue
	}

	// Where a fix is known, from the open findings the claim's rows are
	// about: the same match a row makes when a finding asks whether it
	// applies — the product, the place and both versions — and only the
	// findings the reader may see, since a fix version is read off the
	// finding. A claim covers one component, so one answer per issue is the
	// ordinary case and the smallest stated version stands in otherwise.
	var fixes []struct {
		ClaimID         int64  `bun:"claim_id"`
		VulnerabilityID int64  `bun:"vulnerability_id"`
		FixedIn         string `bun:"fixed_in"`
	}
	known := s.db.NewSelect().
		TableExpr("decision AS de").
		// The decision on the outside, as buildsCovered has it.
		Join("CROSS JOIN finding AS f").
		Where("f.vulnerability_id = de.vulnerability_id AND f.place_identity = de.place_identity").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("de.claim_id AS claim_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("MIN(f.fixed_in) AS fixed_in").
		Where("de.claim_id IN (?)", bun.List(claimIDs)).
		Where("f.closed_run_id IS NULL").
		Where("st.product_id = de.product_id").
		Where("COALESCE(de.component_upstream_version, '') = " + finding.ComponentUpstreamExpr).
		Where("COALESCE(de.consumer_upstream_version, '') = " + finding.ConsumerUpstreamExpr).
		Where("f.fixed_in <> ''")
	if err := readableFindings(known, subject, "f", "st.product_id").
		GroupExpr("de.claim_id, f.vulnerability_id").
		Scan(ctx, &fixes); err != nil {
		return nil, fmt.Errorf("read which of these have a fix: %w", err)
	}
	fixedIn := map[int64]map[int64]string{}
	for _, fix := range fixes {
		if fixedIn[fix.ClaimID] == nil {
			fixedIn[fix.ClaimID] = map[int64]string{}
		}
		fixedIn[fix.ClaimID][fix.VulnerabilityID] = fix.FixedIn
	}

	for _, claim := range claims {
		out[claim.ID] = outliersOf(claim, covers[claim.ID], byIssue, fixedIn[claim.ID])
	}
	return out, nil
}

// outliersOf picks, from the issues one bulk claim covers, the rows that do
// not look like the rest.
func outliersOf(claim Claim, decisionOf map[int64]int64, byIssue map[int64]finding.Vulnerability,
	fixedIn map[int64]string) *Outliers {

	term := narrowingTerm(orEmpty(claim.SelectedBy))
	out := &Outliers{Rows: []Outlier{}}
	candidates := make([]Outlier, 0, len(decisionOf))
	// Walked in issue order so the result is the same on every engine and
	// every run, whatever order a map hands them out in.
	issueIDs := make([]int64, 0, len(decisionOf))
	for id := range decisionOf {
		issueIDs = append(issueIDs, id)
	}
	sort.Slice(issueIDs, func(i, j int) bool { return issueIDs[i] < issueIDs[j] })
	for _, id := range issueIDs {
		issue, known := byIssue[id]
		if !known {
			continue
		}
		word := issue.Severity
		if issue.AssessedSeverity != nil && *issue.AssessedSeverity != "" {
			word = *issue.AssessedSeverity
		}
		if word == "" && issue.ScoreCenti != nil {
			word = finding.SeverityWord(*issue.ScoreCenti)
		}
		one := Outlier{
			DecisionID: decisionOf[issue.ID], Vulnerability: issue.Identifier,
			Severity: word, Exploited: issue.Exploited, FixedIn: fixedIn[issue.ID],
			Description: clip(issue.Description, 200),
		}
		if issue.Exploited {
			out.Exploited++
			one.Why = append(one.Why, "known to be exploited")
		}
		if word == "critical" || word == "high" {
			out.Severe++
			one.Why = append(one.Why, "rated "+word)
		}
		if one.FixedIn != "" {
			out.Fixable++
			one.Why = append(one.Why, "a fix is available in "+one.FixedIn)
		}
		if term != "" && !strings.Contains(strings.ToLower(issue.Description), strings.ToLower(term)) {
			out.Unmatched++
			one.Why = append(one.Why, "does not mention \""+term+"\"")
		}
		if len(one.Why) > 0 {
			candidates = append(candidates, one)
		}
	}
	// Exploited first, then the worst rated, then by name so the order is the
	// same on every engine.
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Exploited != b.Exploited {
			return a.Exploited
		}
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) > severityRank(b.Severity)
		}
		return a.Vulnerability < b.Vulnerability
	})
	if len(candidates) > outlierRows {
		candidates = candidates[:outlierRows]
	}
	out.Rows = candidates
	return out
}

// narrowingTerm reads the term a bulk set was narrowed by, where the record
// of how it was narrowed names one as `contains "term"`.
func narrowingTerm(selectedBy string) string {
	found := containsTerm.FindStringSubmatch(selectedBy)
	if len(found) < 2 {
		return ""
	}
	return strings.TrimSpace(found[1])
}

var containsTerm = regexp.MustCompile(`contains\s+"([^"]+)"`)

func severityRank(word string) int {
	switch word {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// clip shortens text to n characters for a card, on a rune boundary.
func clip(text string, n int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "\u2026"
}

// notApprovableBy narrows a query to the decisions a subject may not agree
// to — the complement of approvableBy, used to ask whether a claim has any
// row outside what the reader may act on.
func notApprovableBy(query *bun.SelectQuery, subject access.Subject, column string) *bun.SelectQuery {
	if subject.Kind != access.Person {
		return query
	}
	products, all := subject.Products()
	if all {
		return query.Where("1 = 0")
	}
	var private, public []int64
	for _, id := range products {
		switch {
		case mayApprove(subject, id, access.Private):
			private = append(private, id)
		case mayApprove(subject, id, access.Public):
			public = append(public, id)
		}
	}
	if len(private) == 0 && len(public) == 0 {
		return query
	}
	return query.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		if len(private) > 0 {
			q = q.Where(column+".product_id NOT IN (?)", bun.List(private))
		}
		if len(public) > 0 {
			q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.WhereOr(column+".product_id NOT IN (?)", bun.List(public)).
					WhereOr(column+".visibility <> ?", access.Public)
			})
		}
		return q
	})
}

// currentReasoning reads the words each claim currently rests on.
// ReasoningFor returns the reasoning each decision currently rests on, keyed
// by decision.
func (s *Store) ReasoningFor(ctx context.Context, decisions []Decision) (map[int64]string, error) {
	return s.currentReasoning(ctx, decisions)
}

func (s *Store) currentReasoning(ctx context.Context, decisions []Decision) (map[int64]string, error) {
	wanted := make([]int64, 0, len(decisions))
	for _, decision := range decisions {
		if decision.RevisionID != nil {
			wanted = append(wanted, *decision.RevisionID)
		}
	}
	if len(wanted) == 0 {
		return map[int64]string{}, nil
	}

	var revisions []Revision
	if err := s.db.NewSelect().Model(&revisions).
		Where("id IN (?)", bun.List(wanted)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read the reasoning: %w", err)
	}
	byDecision := make(map[int64]string, len(revisions))
	for _, revision := range revisions {
		byDecision[revision.DecisionID] = revision.Body
	}
	return byDecision, nil
}

// everApproved reports which of these were agreed to at some point.
func (s *Store) everApproved(ctx context.Context, ids []int64) (map[int64]bool, error) {
	var approvals []Approval
	if err := s.db.NewSelect().Model(&approvals).
		Where("decision_id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what has been agreed to before: %w", err)
	}
	seen := make(map[int64]bool, len(approvals))
	for _, approval := range approvals {
		seen[approval.DecisionID] = true
	}
	return seen, nil
}

// DeferredSoFar is the total time a finding has been put off, across every
// deferral ever recorded about the same place.
//
// Cumulative rather than per deferral, because otherwise deferring repeatedly
// for just under the threshold never needs agreement — and four consecutive
// twenty-nine day deferrals are a year nobody approved.
func (s *Store) DeferredSoFar(ctx context.Context, decision Decision) (time.Duration, error) {
	decision.ID = -1
	totals, err := s.deferredSoFar(ctx, []Decision{decision})
	if err != nil {
		return 0, err
	}
	return totals[decision.ID], nil
}

// deferredSoFar reads, in one statement for all of them, how long each of
// these decisions' places has been put off for in total, keyed by the
// decision asked about.
//
// One statement for the page rather than one per row: the deferrals in the
// products on the page are read together and matched to each decision's
// place here. The set is small — a deferral is a decision of ours, in these
// products, and most decisions are not deferrals — where a lookup per row
// was a statement per queue entry.
func (s *Store) deferredSoFar(ctx context.Context, decisions []Decision) (map[int64]time.Duration, error) {
	totals := make(map[int64]time.Duration, len(decisions))
	if len(decisions) == 0 {
		return totals, nil
	}
	products := map[int64]bool{}
	issues := map[int64]bool{}
	for _, decision := range decisions {
		products[decision.ProductID] = true
		issues[decision.VulnerabilityID] = true
	}
	productIDs := make([]int64, 0, len(products))
	for id := range products {
		productIDs = append(productIDs, id)
	}
	issueIDs := make([]int64, 0, len(issues))
	for id := range issues {
		issueIDs = append(issueIDs, id)
	}

	var deferrals []Decision
	if err := s.db.NewSelect().Model(&deferrals).
		Where("product_id IN (?)", bun.List(productIDs)).
		Where("vulnerability_id IN (?)", bun.List(issueIDs)).
		Where("outcome = ?", Deferred).
		// What was taken back was not time the finding spent put off. Counting
		// a withdrawn deferral would make the number shown to an approver —
		// "how long has this been postponed" — include time it was not.
		Where("state <> ?", Withdrawn).
		Where("deferred_until IS NOT NULL").Scan(ctx); err != nil {
		return nil, fmt.Errorf("read how long these have been put off: %w", err)
	}

	type place struct {
		product, issue int64
		at             string
	}
	spans := map[place]time.Duration{}
	for _, deferral := range deferrals {
		if deferral.DeferredUntil == nil {
			continue
		}
		// Measured from when it was asked for, so a deferral that has not yet
		// run out counts the whole of what it asked for rather than only the
		// part already spent. The question is how long this has been put off
		// for, not how long it has been put off so far.
		if span := deferral.DeferredUntil.Sub(deferral.ProposedAt); span > 0 {
			spans[place{deferral.ProductID, deferral.VulnerabilityID, deferral.PlaceIdentity}] += span
		}
	}
	for _, decision := range decisions {
		totals[decision.ID] = spans[place{decision.ProductID, decision.VulnerabilityID, decision.PlaceIdentity}]
	}
	return totals, nil
}

// NeedsApproval reports whether a proposal may stand on its own.
//
// Hiding risk needs a second person. The exception is a short deferral: a
// quick "not this sprint" is ordinary triage and gating it would put every
// routine act through a queue, which is how a queue stops being read.
//
// "Short" is measured against everything this finding has already been put off
// for. Otherwise the exception swallows the rule one twenty-nine day deferral
// at a time.
func (s *Store) NeedsApproval(ctx context.Context, p Proposal, threshold time.Duration) (bool, error) {
	if !p.Outcome.HidesRisk() {
		return false, nil
	}
	if p.Outcome != Deferred {
		return true, nil
	}
	if threshold <= 0 {
		return true, nil
	}

	asking := time.Duration(0)
	if p.DeferredUntil != nil {
		// Measured from this store's own clock, like every other time decision
		// here, and never negative. A date already past asks for no time at
		// all; letting it come out negative would let a back-dated deferral
		// subtract from what a finding has already been postponed for and slip
		// under the threshold.
		if span := p.DeferredUntil.Sub(s.now()); span > 0 {
			asking = span
		}
	}

	// What has already been asked for about this same place.
	already, err := s.DeferredSoFar(ctx, Decision{
		ProductID: p.Place.ProductID, VulnerabilityID: p.Place.VulnerabilityID,
		PlaceIdentity: p.Place.PlaceIdentity,
	})
	if err != nil {
		return false, err
	}
	return already+asking >= threshold, nil
}

// waiting narrows a query to what somebody has to look at.
//
// Three things, not one. A claim awaiting agreement is the obvious case. The
// other two are what happens when a judgment stops covering anything:
//
// A deferral that has run out has said what it was going to say. The finding
// is back, and if it does not appear here it simply reappears as new with the
// reasoning stranded behind it — which is the outcome marking a lapse exists
// to prevent.
//
// A decision the code moved out from under is the same shape: somebody made a
// judgment, it no longer applies, and they are the person who should be told.
//
// A claim that needed nobody — a short deferral — is not here at all. A work
// list containing work nobody has to do teaches people to skip rows.
func waiting(query *bun.SelectQuery, now time.Time) *bun.SelectQuery {
	return query.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.
			WhereOr("state = ? AND needs_approval = ? AND sent_back_at IS NULL", Proposed, true).
			WhereOr("state = ?", LapsedState).
			WhereOr("state IN (?, ?) AND outcome = ? AND deferred_until IS NOT NULL AND deferred_until <= ?",
				Proposed, Approved, Deferred, now)
	})
}
