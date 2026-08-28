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
	"strings"
	"time"

	"github.com/uptrace/bun"
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
	Name     string `bun:"name,notnull"`
	Version  string `bun:"version,notnull"`
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
func (d Described) Identity() string {
	basis := strings.TrimSpace(d.Purl)
	if basis == "" {
		basis = strings.TrimSpace(d.Name) + "@" + strings.TrimSpace(d.Version)
	}
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

// Valid reports whether a described component can be stored.
func (d Described) Valid() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("component has no name")
	}
	if strings.TrimSpace(d.Version) == "" {
		return fmt.Errorf("component %q has no version", d.Name)
	}
	return nil
}

// Components reads and writes the shared component catalogue.
type Components struct {
	db  bun.IDB
	now func() time.Time
}

// NewComponents returns a catalogue over db.
func NewComponents(db bun.IDB) *Components {
	return &Components{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Intern returns the identifiers for these components, inserting any that are
// new.
//
// Deduplication is the point. The same library at the same version is one row
// however many products ship it; without that, a component shared across a
// portfolio is stored once per variant per scan and the table grows with the
// catalogue rather than with reality.
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
			Identity: identity, Purl: d.Purl, Name: d.Name, Version: d.Version,
			UpstreamName: d.UpstreamName, UpstreamVersion: d.UpstreamVersion,
			FirstSeenAt: now,
		})
	}
	if len(missing) > 0 {
		if _, err := c.db.NewInsert().Model(&missing).Exec(ctx); err != nil {
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
