// Package graph stores the dependency graph a scan describes, and the
// components in it.
//
// The graph is held as nodes and edges with validity intervals, not as one set
// of rows per scan. A nightly rebuild changes very little, so recording only
// what changed keeps stored volume tracking change rather than tracking scans
// — which is the difference between a table that grows with real events and
// one that grows with the calendar.
package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Component is a package at a version, shared across every product that ships
// it.
type Component struct {
	bun.BaseModel `bun:"table:component,alias:c"`

	ID int64 `bun:"id,pk,autoincrement"`
	// Identity is derived from the component's own content, never from an
	// identifier the scan file supplied. Nothing guarantees those are stable
	// between builds or consistent between producers, and an identity that
	// moves would reset every triage decision attached to it.
	Identity string `bun:"identity,notnull"`
	Purl     string `bun:"purl"`
	// CPE is the other identifier scheme in circulation. It is not part of
	// identity — that is derived from the package identifier where there is
	// one, and a second basis would move the identity of everything carrying
	// both — but it is what the national vulnerability database keys on, so a
	// scanner given it matches things a package identifier alone misses.
	CPE     string `bun:"cpe"`
	Name    string `bun:"name,notnull"`
	Version string `bun:"version,notnull"`
	// UpstreamName and UpstreamVersion carry what a patched fork was forked
	// from. A shipped build often carries a version string of its own, and the
	// vulnerability identity lives on the upstream one — so dropping this
	// makes findings unexplainable, and it is what expiry compares.
	UpstreamName    string    `bun:"upstream_name"`
	UpstreamVersion string    `bun:"upstream_version"`
	FirstSeenAt     time.Time `bun:"first_seen_at,notnull"`
}

// Described is a component as a scan describes it, before it has been matched
// to a row.
type Described struct {
	Purl            string
	CPE             string
	Name            string
	Version         string
	UpstreamName    string
	UpstreamVersion string
}

// Identity returns the content-derived key for a described component.
//
// The package identifier is used when there is one, because it already
// encodes ecosystem, name and version unambiguously. Where a producer emits
// none — and some do not — name and version stand in. Hashing keeps the key a
// fixed width whatever the identifier's length, which some engines need for an
// index and all of them benefit from.
//
// The identifier is reduced to the parts that say what a package *is* before
// it is hashed. A real inventory spells one package several ways, and taking
// the identifier verbatim makes each spelling a component of its own.
func (d Described) Identity() string {
	basis := canonicalPurl(d.Purl)
	if basis == "" {
		basis = strings.TrimSpace(d.Name) + "@" + strings.TrimSpace(d.Version)
	}
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

// canonicalPurl reduces a package identifier to what identifies the package.
//
// Three reductions, each for something a real inventory does.
//
// **Qualifiers are dropped.** They qualify rather than identify — an
// architecture, a distribution, the source package a binary came from — and a
// build that merges two sources emits one package with them and the same
// package without. Measured on a public switch operating-system image: 8,373
// identifiers spelling 7,857 packages, and every one of the 516 collisions was
// the same name at the same version. None merged a different package.
//
// Architecture is the one that looks like it belongs and does not. What a
// product is built as is already a dimension of the model — a variant — so
// putting it in a component's identity states it twice, and the same package
// then reads as two in a report that has already separated them by variant. In
// this image the two spellings even disagree about it: one source called a
// package "all" and the other "amd64".
//
// **Escapes are decoded.** The same version arrives as `2.3.2-2%2Bb1` and
// `2.3.2-2+b1` from the two sources, which byte comparison calls two packages.
//
// **The type is lowercased**, which the specification requires and which
// nothing else here relies on.
func canonicalPurl(purl string) string {
	purl = strings.TrimSpace(purl)
	if purl == "" {
		return ""
	}

	// Cut before decoding. A name or version may legitimately contain an
	// escaped separator, and decoding first would turn it into one.
	if i := strings.IndexAny(purl, "?#"); i >= 0 {
		purl = purl[:i]
	}

	scheme, rest, found := strings.Cut(purl, ":")
	if !found {
		return decoded(purl)
	}
	// The type is the first segment after the scheme, not the scheme itself,
	// and it is the part the specification calls case-insensitive.
	kind, path, split := strings.Cut(rest, "/")
	if !split {
		return strings.ToLower(scheme) + ":" + decoded(rest)
	}
	return strings.ToLower(scheme) + ":" + strings.ToLower(kind) + "/" + decoded(path)
}

// UpstreamFromPurl reads what a package identifier says it was built from.
//
// Producers state this two ways and mean the same thing. The format has a
// place for it — a pedigree naming what a component descends from — and
// several producers instead hang it off the identifier as a qualifier, which
// is where a distribution's source package ends up. Measured on a public
// switch operating-system image: 30 components state it the first way and 535
// the second, with no overlap at all. Reading only the first captures a
// twentieth of it.
//
// It matters more than its size suggests. A shipped package usually carries a
// version of its own while the vulnerability lives on what it was built from
// (MDL-04), so this is what a finding is explained by and what expiry
// compares. It is also the name a build's own suppressions use, because a
// patch is written against a source tree rather than against the binaries cut
// from it.
//
// The qualifier comes as a bare name or as name and version, and both occur —
// 459 and 76 in that image. A bare name is not a lesser answer: for a binary
// cut from a differently named source package it is the whole of what is
// knowable, and it is the half that matching a claim needs.
func UpstreamFromPurl(purl string) (name, version string) {
	_, qs, found := strings.Cut(strings.TrimSpace(purl), "?")
	if !found || qs == "" {
		return "", ""
	}
	// The subpath, if any, follows the qualifiers and is not one of them.
	qs, _, _ = strings.Cut(qs, "#")

	for _, pair := range strings.Split(qs, "&") {
		key, value, found := strings.Cut(pair, "=")
		if !found || !strings.EqualFold(key, "upstream") {
			continue
		}
		stated := decoded(value)
		// A version, where one is stated. Cut from the right: a name may
		// contain no "@", but a version can, and the separator is the last.
		if at := strings.LastIndex(stated, "@"); at > 0 {
			return stated[:at], stated[at+1:]
		}
		return stated, ""
	}
	return "", ""
}

// decoded resolves percent-escapes, leaving the text alone where they are
// malformed — a producer's identifier is not ours to reject over spelling.
func decoded(s string) string {
	unescaped, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return unescaped
}

// AsRoot returns how the product itself is stored: its name, and nothing that
// moves.
//
// The version is dropped deliberately. It changes on every build, and identity
// is derived from what a component is — so keeping it would give the product a
// new identity every night, close the node standing for it, and close and
// reopen every edge hanging off it. A build in which nothing changed would
// write thousands of rows, which is the one thing the interval shape exists to
// prevent.
//
// Which build this was is not lost by dropping it. That is what the scan
// record holds: when it was built, what it hashed to, and who sent it.
func (d Described) AsRoot() Described { return Described{Name: d.Name} }

// Valid reports whether a described component can be stored.
//
// A name is the whole requirement. A version is not: the format only requires
// a type and a name, so a component without one is ordinary output rather than
// a broken file, and refusing it would throw away every other component in the
// document alongside it.
//
// What a component with no version costs is matching — nothing can say whether
// a vulnerability applies to a version nobody stated. It still ships, so it is
// better held and visible than dropped.
func (d Described) Valid() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("component has no name")
	}
	return nil
}

// Components reads and writes the shared component catalog.
type Components struct {
	db  bun.IDB
	now func() time.Time
}

// NewComponents returns a catalog over db.
func NewComponents(db bun.IDB) *Components {
	return &Components{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Intern returns the identifiers for these components, inserting any that are
// new.
//
// Deduplication is the point. The same library at the same version is one row
// however many products ship it; without that, a component shared across a
// portfolio is stored once per variant per scan and the table grows with the
// catalog rather than with reality.
func (c *Components) Intern(ctx context.Context, described []Described) (map[string]int64, error) {
	byIdentity := make(map[string]Described, len(described))
	for _, d := range described {
		if err := d.Valid(); err != nil {
			return nil, err
		}
		byIdentity[d.Identity()] = d
	}
	if len(byIdentity) == 0 {
		return map[string]int64{}, nil
	}

	identities := make([]string, 0, len(byIdentity))
	for identity := range byIdentity {
		identities = append(identities, identity)
	}

	known, err := c.byIdentities(ctx, identities)
	if err != nil {
		return nil, err
	}

	var missing []Component
	now := c.now().Truncate(time.Microsecond)
	for identity, d := range byIdentity {
		if _, have := known[identity]; have {
			continue
		}
		missing = append(missing, Component{
			Identity: identity, Purl: d.Purl, CPE: d.CPE, Name: d.Name, Version: d.Version,
			UpstreamName: d.UpstreamName, UpstreamVersion: d.UpstreamVersion,
			FirstSeenAt: now,
		})
	}
	if len(missing) > 0 {
		if err := database.InBatches(ctx, c.db, missing); err != nil {
			return nil, fmt.Errorf("record %d new components: %w", len(missing), err)
		}
		for _, added := range missing {
			known[added.Identity] = added.ID
		}
	}
	return known, nil
}

// byIdentities looks up components in batches.
//
// Batched because a scan can describe tens of thousands of components, and one
// query with that many bound parameters exceeds what some engines accept.
func (c *Components) byIdentities(ctx context.Context, identities []string) (map[string]int64, error) {
	const batch = 500
	found := make(map[string]int64, len(identities))

	for start := 0; start < len(identities); start += batch {
		end := min(start+batch, len(identities))

		var rows []Component
		err := c.db.NewSelect().Model(&rows).
			Column("id", "identity").
			Where("identity IN (?)", bun.List(identities[start:end])).
			Scan(ctx)
		if err != nil {
			return nil, fmt.Errorf("look up components: %w", err)
		}
		for _, row := range rows {
			found[row.Identity] = row.ID
		}
	}
	return found, nil
}

// FillFrom takes into a description anything it does not state and another
// description of the same component does.
//
// Nothing already stated is overwritten. Two producers describing one package
// differently is not something this can adjudicate, and the first answer is
// the one everything downstream has already been given — so the later one
// fills gaps and nothing else.
func (d *Described) FillFrom(other Described) {
	if d.CPE == "" {
		d.CPE = other.CPE
	}
	if d.Version == "" {
		d.Version = other.Version
	}
	if d.UpstreamName == "" {
		d.UpstreamName, d.UpstreamVersion = other.UpstreamName, other.UpstreamVersion
	} else if d.UpstreamVersion == "" && d.UpstreamName == other.UpstreamName {
		d.UpstreamVersion = other.UpstreamVersion
	}
}
