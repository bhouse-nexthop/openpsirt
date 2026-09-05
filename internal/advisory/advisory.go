// Package advisory turns what is held about a flaw in our own product into a
// document somebody can publish.
//
// **We own the triage record; whoever publishes owns the published advisory**
// (REM-18). Nothing here records that a document was issued, and nothing goes
// out over the network: the document is assembled from what is held and handed
// over. That is the question that decides whether an integration works or
// rots, and keeping both ends as the source of truth is how it rots.
//
// **Only a flaw in what we ship** (REM-23). A known issue in a third-party
// component is dependency hygiene that a consumer can already read out of the
// inventory, and issuing a vendor advisory for every upstream CVE in a
// dependency is not what an advisory is. So this refuses an issue this
// deployment did not record, by name, rather than producing a document that
// looks the same and means something else.
package advisory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/version"
)

// Publisher is who the document says issued it.
//
// Configured per deployment (REM-20) rather than tuned by an administrator: it
// is the identity of the organization running this, which is a property of the
// installation in the same way the address people arrive on is.
type Publisher struct {
	Name      string
	Namespace string
	// Category is what CSAF calls the kind of publisher. A deployment
	// publishing about its own product is a vendor, which is why that is the
	// default and why nothing here works it out from anything.
	Category string
}

// Stated reports whether enough is configured to name a publisher.
//
// A document with no publisher is not a CSAF document — the field is required
// — so an unconfigured deployment is told that rather than handed something
// that fails validation wherever it is taken next.
func (p Publisher) Stated() bool { return p.Name != "" && p.Namespace != "" }

// ErrNotOurs says the issue is not one this deployment recorded.
var ErrNotOurs = errors.New(
	"an advisory is about a flaw in what we ship, and this issue was reported by a scanner")

// ErrNoPublisher says the deployment has not been told who it publishes as.
//
// Wrapped by missingPublisher, which names the variable that is not set: the
// person who sees this cannot fix it, and the operator who can is reading a
// relayed message rather than sitting at the process.
var ErrNoPublisher = errors.New("this deployment has not been configured with a publisher")

// missingPublisher says which half of the publisher is missing.
func missingPublisher(p Publisher) error {
	switch {
	case p.Name == "" && p.Namespace == "":
		return fmt.Errorf("%w: set %sPUBLISHER_NAME and %sPUBLISHER_NAMESPACE",
			ErrNoPublisher, envPrefix, envPrefix)
	case p.Name == "":
		return fmt.Errorf("%w: %sPUBLISHER_NAME is not set", ErrNoPublisher, envPrefix)
	default:
		return fmt.Errorf("%w: %sPUBLISHER_NAMESPACE is not set", ErrNoPublisher, envPrefix)
	}
}

// envPrefix is how the settings are spelled in the environment, repeated here
// rather than imported: the configuration package reads this one, and a cycle
// for the sake of a five-character string is a worse trade than the string.
const envPrefix = "OPENPSIRT_"

// ErrNoSuchIssue says the product holds nothing under that identifier.
var ErrNoSuchIssue = errors.New("this product holds no issue by that name")

// Document is a CSAF 2.0 document.
//
// The field names and their shapes are the standard's, not ours, so they are
// spelled as it spells them and are exempt from this codebase's spelling rule
// for the same reason a producer's field names are.
type Document struct {
	Document    Meta        `json:"document"`
	ProductTree ProductTree `json:"product_tree"`
	// Vulnerabilities holds one entry. An advisory aggregates a product and a
	// version range rather than a path (REM-19), and this is about one flaw.
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

// Meta is the document's own description.
type Meta struct {
	// Category is what kind of document this is. It follows what the document
	// can actually support rather than what would sound better: the VEX
	// profile is the one that carries "not affected, and here is why", and
	// those justifications are not assembled here yet.
	Category    string   `json:"category"`
	CSAFVersion string   `json:"csaf_version"`
	Title       string   `json:"title"`
	Publisher   Issuer   `json:"publisher"`
	Tracking    Tracking `json:"tracking"`
	Notes       []Note   `json:"notes,omitempty"`
	Language    string   `json:"lang,omitempty"`
}

// Issuer is the publisher as the document carries it.
type Issuer struct {
	Category  string `json:"category"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// Tracking is the document's identity and where it is in its life.
type Tracking struct {
	ID string `json:"id"`
	// Status is draft while nobody outside has been told.
	//
	// Reaching a disclosure date discloses nothing (ACC-47) — it escalates,
	// and a person decides — so a document about an undisclosed flaw is
	// prepared rather than issued, and says so in the one field a reader of a
	// CSAF document checks before acting on it.
	Status             string     `json:"status"`
	Version            string     `json:"version"`
	InitialReleaseDate time.Time  `json:"initial_release_date"`
	CurrentReleaseDate time.Time  `json:"current_release_date"`
	Generator          *Generator `json:"generator,omitempty"`
	RevisionHistory    []Revision `json:"revision_history"`
}

// Generator names what assembled the document.
type Generator struct {
	Engine Engine    `json:"engine"`
	Date   time.Time `json:"date"`
}

// Engine is the software that generated it.
type Engine struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Revision is one entry in the document's history.
type Revision struct {
	Number  string    `json:"number"`
	Date    time.Time `json:"date"`
	Summary string    `json:"summary"`
}

// Note is prose attached to a document or a vulnerability.
type Note struct {
	Category string `json:"category"`
	Title    string `json:"title,omitempty"`
	Text     string `json:"text"`
}

// ProductTree names everything the document can make a statement about.
type ProductTree struct {
	Branches []Branch `json:"branches,omitempty"`
}

// Branch is one level of that naming.
type Branch struct {
	Category string   `json:"category"`
	Name     string   `json:"name"`
	Branches []Branch `json:"branches,omitempty"`
	Product  *Named   `json:"product,omitempty"`
}

// Named is a leaf of the product tree: something a status can be stated about.
type Named struct {
	Name string `json:"name"`
	ID   string `json:"product_id"`
}

// Vulnerability is the flaw and what is true of it in each release.
type Vulnerability struct {
	// CVE where it has one, and IDs otherwise. An identifier this deployment
	// minted is not a CVE and saying it is in that field would be a claim
	// nobody assigned (MDL-24).
	CVE   string   `json:"cve,omitempty"`
	IDs   []Issued `json:"ids,omitempty"`
	Title string   `json:"title,omitempty"`
	Notes []Note   `json:"notes,omitempty"`
	// Status is which releases the flaw is in and which it is out of.
	Status Status `json:"product_status"`
	// DiscoveryDate is when this deployment first recorded it, which is what
	// it knows. When somebody outside found it is not something it holds.
	DiscoveryDate string `json:"discovery_date,omitempty"`
}

// Issued is an identifier somebody else's system knows this by.
type Issued struct {
	SystemName string `json:"system_name"`
	Text       string `json:"text"`
}

// Status is which releases the flaw is in and which it is out of.
//
// A release that held the flaw and no longer does is named as fixed rather
// than left out, because leaving it out reads identically to a release that
// never shipped the thing at all — and those are opposite answers, one of them
// the one a reader is hoping for.
//
// What a person decided about a release — not affected, and the reason why —
// is the VEX half, and is not assembled here.
type Status struct {
	KnownAffected []string `json:"known_affected,omitempty"`
	Fixed         []string `json:"fixed,omitempty"`
}

// Store assembles advisories.
type Store struct {
	db  *bun.DB
	now func() time.Time
}

// NewStore returns a store over db.
func NewStore(db *bun.DB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// For assembles the advisory for one issue in one product.
func (s *Store) For(ctx context.Context, subject access.Subject, publisher Publisher,
	product, identifier string) (*Document, error) {

	if !publisher.Stated() {
		return nil, missingPublisher(publisher)
	}
	named, err := catalog.NewStore(s.db).ProductByName(ctx, product)
	if err != nil {
		return nil, err
	}
	// Authorized before the identifier is resolved, so a name nobody holds and
	// a name somebody holds come back the same way (ACC-56).
	if subject.Kind != access.Person || !subject.Sees(named.ID) {
		return nil, ErrNoSuchIssue
	}

	issue, entered, err := s.ours(ctx, subject, named.ID, identifier)
	if err != nil {
		return nil, err
	}

	releases, err := s.releases(ctx, subject, named.ID, issue.ID)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	shown := named.DisplayName
	if shown == "" {
		shown = named.Name
	}

	doc := &Document{}
	doc.Document = Meta{
		// A security advisory rather than the VEX profile, because what this
		// carries is which releases are affected and which are fixed. The VEX
		// profile's point is the not-affected justification, and claiming that
		// category while carrying none of them would describe the document as
		// something it is not.
		Category:    "csaf_security_advisory",
		CSAFVersion: "2.0",
		Title:       fmt.Sprintf("%s: %s", shown, summaryOf(issue, identifier)),
		Language:    "en-US",
		Publisher: Issuer{
			Category: categoryOf(publisher), Name: publisher.Name,
			Namespace: publisher.Namespace,
		},
		Tracking: Tracking{
			ID: identifier, Status: statusOf(entered), Version: "1",
			InitialReleaseDate: entered.OpenedAt.UTC(),
			CurrentReleaseDate: now,
			// Which build wrote it, read from the binary rather than held in
			// a variable something has to remember to set — one nobody set
			// says the document was generated by a version that does not
			// exist.
			Generator: &Generator{
				Engine: Engine{Name: "OpenPSIRT", Version: version.Get().Version},
				Date:   now,
			},
			RevisionHistory: []Revision{{
				Number: "1", Date: entered.OpenedAt.UTC(),
				Summary: "Recorded in OpenPSIRT",
			}},
		},
	}
	if text := summaryOf(issue, identifier); text != "" {
		doc.Document.Notes = []Note{{Category: "description", Title: "Summary", Text: text}}
	}

	vulnerability := Vulnerability{
		Title: summaryOf(issue, identifier),
		IDs:   []Issued{{SystemName: publisher.Name, Text: identifier}},
	}
	// A CVE assigned later is another name for the same issue, and the issue
	// is then filed under it (MDL-24). Where that has happened the document
	// says so in the field a reader looks in.
	if isCVE(issue.Identifier) {
		vulnerability.CVE = issue.Identifier
	}
	if !entered.OpenedAt.IsZero() {
		vulnerability.DiscoveryDate = entered.OpenedAt.UTC().Format("2006-01-02")
	}

	// One branch per release, under the product, under the publisher. The
	// tree names releases rather than components on purpose: an advisory
	// aggregates to a product and a version range, and a reader of one is
	// asking "am I affected", which a dependency path does not answer
	// (REM-19).
	versions := make([]Branch, 0, len(releases))
	for _, release := range releases {
		versions = append(versions, Branch{
			Category: "product_version", Name: release.Name(),
			Product: &Named{
				Name: fmt.Sprintf("%s %s", shown, release.Name()),
				ID:   release.ProductID(product),
			},
		})
		if release.Holds {
			vulnerability.Status.KnownAffected = append(
				vulnerability.Status.KnownAffected, release.ProductID(product))
		} else {
			vulnerability.Status.Fixed = append(
				vulnerability.Status.Fixed, release.ProductID(product))
		}
	}
	doc.ProductTree = ProductTree{Branches: []Branch{{
		Category: "vendor", Name: publisher.Name,
		Branches: []Branch{{
			Category: "product_name", Name: shown, Branches: versions,
		}},
	}}}
	doc.Vulnerabilities = []Vulnerability{vulnerability}
	return doc, nil
}

// Release is one build of the product and where it stands on the issue.
type Release struct {
	Stream  string
	Variant string
	// Holds says the issue is open there. False is a release that held it and
	// no longer does, which is the one that was fixed.
	Holds bool
}

// Name is how the release is written in the document.
func (r Release) Name() string { return r.Stream + " (" + r.Variant + ")" }

// ProductID is the identifier statements refer to it by. A release is named by
// its stream and its variant together, never by one of them: the same branch
// built two ways is two builds, and naming only the branch would claim
// something about hardware nobody built for.
func (r Release) ProductID(product string) string {
	return product + ":" + r.Stream + ":" + r.Variant
}

// ours reads the issue and the finding this deployment recorded for it, and
// refuses one that a scanner reported.
func (s *Store) ours(ctx context.Context, subject access.Subject, productID int64,
	identifier string) (*finding.Vulnerability, *finding.Finding, error) {

	var issue finding.Vulnerability
	err := s.db.NewSelect().Model(&issue).
		Where("identifier = ?", identifier).
		Limit(1).Scan(ctx)
	if err != nil {
		return nil, nil, ErrNoSuchIssue
	}

	// The earliest finding of this issue in this product that a person
	// recorded. Earliest because it is what the document dates itself from,
	// and a flaw recorded once and later found in a second release is one
	// flaw with one discovery.
	var row finding.Finding
	err = s.db.NewSelect().Model(&row).
		Join("JOIN target AS t ON t.id = f.target_id").
		Join("JOIN stream AS st ON st.id = t.stream_id").
		Where("st.product_id = ?", productID).
		Where("f.vulnerability_id = ?", issue.ID).
		Where("f.visibility IN (?)", bun.List(access.Visible(subject, productID))).
		Where("f.kind = ?", finding.Entered).
		OrderExpr("f.opened_at ASC, f.id ASC").
		Limit(1).Scan(ctx)
	if err != nil {
		// Whether the issue is here at all and whether it is ours are told
		// apart deliberately: the first is a typo and the second is a scope
		// rule somebody has to understand.
		if s.here(ctx, subject, productID, issue.ID) {
			return nil, nil, ErrNotOurs
		}
		return nil, nil, ErrNoSuchIssue
	}
	return &issue, &row, nil
}

// here reports whether the product holds this issue at all, however it arrived.
func (s *Store) here(ctx context.Context, subject access.Subject,
	productID, issueID int64) bool {

	count, err := s.db.NewSelect().Model((*finding.Finding)(nil)).
		Join("JOIN target AS t ON t.id = f.target_id").
		Join("JOIN stream AS st ON st.id = t.stream_id").
		Where("st.product_id = ?", productID).
		Where("f.vulnerability_id = ?", issueID).
		Where("f.visibility IN (?)", bun.List(access.Visible(subject, productID))).
		Count(ctx)
	return err == nil && count > 0
}

// releases reports every build of the product that holds this issue or once
// did, which is what an advisory states something about.
func (s *Store) releases(ctx context.Context, subject access.Subject,
	productID, issueID int64) ([]Release, error) {

	var rows []struct {
		Stream  string `bun:"stream"`
		Variant string `bun:"variant"`
		Open    int    `bun:"open"`
	}
	// One statement rather than one per build: a product with thirty tags
	// would otherwise be thirty round trips to write one document, and the
	// answer would be assembled from thirty moments rather than one.
	err := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS t ON t.id = f.target_id").
		Join("JOIN stream AS st ON st.id = t.stream_id").
		Join("JOIN variant AS va ON va.id = t.variant_id").
		ColumnExpr("st.name AS stream").
		ColumnExpr("va.name AS variant").
		// Counted rather than filtered, so a release that held the flaw and no
		// longer does is still a row — that is the release somebody upgrades
		// to, and dropping it would leave finished work indistinguishable
		// from a release that never shipped the thing.
		ColumnExpr("COUNT(CASE WHEN f.closed_at IS NULL THEN 1 END) AS open").
		Where("st.product_id = ?", productID).
		Where("f.vulnerability_id = ?", issueID).
		Where("f.visibility IN (?)", bun.List(access.Visible(subject, productID))).
		GroupExpr("st.name, va.name").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read which releases this is in: %w", err)
	}

	releases := make([]Release, 0, len(rows))
	for _, row := range rows {
		releases = append(releases, Release{
			Stream: row.Stream, Variant: row.Variant, Holds: row.Open > 0,
		})
	}
	// Ordered here rather than by the engine, so the document is byte-for-byte
	// the same whatever it was generated against — which is what lets somebody
	// diff two of them and see a real change.
	sort.Slice(releases, func(i, j int) bool {
		if releases[i].Stream != releases[j].Stream {
			return releases[i].Stream < releases[j].Stream
		}
		return releases[i].Variant < releases[j].Variant
	})
	return releases, nil
}

// statusOf says where the document sits in its life.
func statusOf(row *finding.Finding) string {
	if row.Visibility == access.Private {
		return "draft"
	}
	return "final"
}

func categoryOf(p Publisher) string {
	if p.Category == "" {
		return "vendor"
	}
	return p.Category
}

// summaryOf is what the flaw is, in the words of whoever recorded it.
func summaryOf(issue *finding.Vulnerability, identifier string) string {
	if issue.Description != "" {
		return issue.Description
	}
	return identifier
}

// isCVE says the issue is filed under a CVE rather than under an identifier
// this deployment minted.
func isCVE(identifier string) bool {
	return len(identifier) > 4 && identifier[:4] == "CVE-"
}
