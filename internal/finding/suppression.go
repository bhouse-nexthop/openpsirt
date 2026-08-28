package finding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
)

// Claim is one thing a build argued about one vulnerability in one of the
// things it ships.
//
// A statement naming several packages becomes several of these. Each row is a
// claim about one subject, because that is what has to be matched against a
// component and because a claim that reached one package and not another is a
// thing worth being able to see.
type Claim struct {
	bun.BaseModel `bun:"table:suppression,alias:sup"`

	ID       int64  `bun:"id,pk,autoincrement"`
	TargetID int64  `bun:"target_id,notnull"`
	Identity string `bun:"identity,notnull"`
	// Vulnerability is the identifier the build argued about, as it wrote it.
	Vulnerability string `bun:"vulnerability,notnull"`
	Status        string `bun:"status,notnull"`
	Justification string `bun:"justification"`
	Statement     string `bun:"statement"`
	// Origin says whether this came attached to a component or in a document
	// of its own, which is the difference between a claim that knows exactly
	// what it is about and one that names something we have to match.
	Origin       string `bun:"origin,notnull"`
	SubjectPurl  string `bun:"subject_purl"`
	SubjectName  string `bun:"subject_name"`
	OpenedScanID int64  `bun:"opened_scan_id,notnull"`
	ClosedScanID *int64 `bun:"closed_scan_id"`
}

// covers reports whether this claim is about the component described.
func (c Claim) covers(d graph.Described) bool {
	return sbom.Target{Purl: c.SubjectPurl, Name: c.SubjectName}.Covers(d)
}

// suppresses reports whether the claim removes a finding from what somebody
// has to look at. A build saying it is affected, or that it has not decided,
// is information rather than an answer.
func (c Claim) suppresses() bool { return sbom.Status(c.Status).Suppresses() }

// claimIdentity derives a stable key from what a claim says.
//
// Everything that makes the claim a different claim is in it, so re-sending
// the same argument writes nothing and changing the reasoning is a change.
func claimIdentity(c Claim) string {
	basis := strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(c.Vulnerability)),
		c.Status, c.Justification, c.Origin, c.SubjectPurl, c.SubjectName,
	}, "\x00")
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

// ClaimsApplied describes what recording a build's claims changed.
type ClaimsApplied struct {
	Opened int
	Closed int
}

// Unchanged reports whether a build argued exactly what it argued last time.
func (a ClaimsApplied) Unchanged() bool { return a.Opened == 0 && a.Closed == 0 }

// RecordClaims stores what a build argued, writing only the difference.
//
// Kept as data rather than left in the document it arrived in: a nightly
// scan's documents are discarded once read, the scan itself runs later, and it
// runs again on a schedule. A claim that lived only in the file would be gone
// by the time anything needed it, and every carried patch would come back as
// an outstanding vulnerability on the next re-scan.
func (s *Store) RecordClaims(ctx context.Context, targetID, scanID int64, claims []sbom.Suppression) (ClaimsApplied, error) {
	var applied ClaimsApplied

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		wanted := map[string]Claim{}
		for _, claim := range claims {
			for _, subject := range claim.Targets {
				row := Claim{
					TargetID: targetID, Vulnerability: claim.Vulnerability,
					Status: string(claim.Status), Justification: claim.Justification,
					Statement: claim.Statement, Origin: string(claim.Origin),
					SubjectPurl: subject.Purl, SubjectName: subject.Name,
					OpenedScanID: scanID,
				}
				row.Identity = claimIdentity(row)
				wanted[row.Identity] = row
			}
		}

		var open []Claim
		err := tx.NewSelect().Model(&open).
			Where("target_id = ?", targetID).Where("closed_scan_id IS NULL").Scan(ctx)
		if err != nil {
			return fmt.Errorf("read what this build argued before: %w", err)
		}

		held := map[string]int64{}
		for _, row := range open {
			held[row.Identity] = row.ID
		}

		var opening []Claim
		for identity, row := range wanted {
			if _, already := held[identity]; !already {
				opening = append(opening, row)
			}
		}
		if len(opening) > 0 {
			if err := database.InBatches(ctx, tx, opening); err != nil {
				return fmt.Errorf("record %d claims: %w", len(opening), err)
			}
			applied.Opened = len(opening)
		}

		var closing []int64
		for identity, id := range held {
			if _, still := wanted[identity]; !still {
				closing = append(closing, id)
			}
		}
		if len(closing) > 0 {
			err := database.IDsInBatches(ctx, closing, func(ctx context.Context, batch []int64) error {
				_, err := tx.NewUpdate().Model((*Claim)(nil)).
					Set("closed_scan_id = ?", scanID).
					Where("id IN (?)", bun.List(batch)).Exec(ctx)
				return err
			})
			if err != nil {
				return fmt.Errorf("close %d claims: %w", len(closing), err)
			}
			applied.Closed = len(closing)
		}
		return nil
	})
	return applied, err
}

// OpenClaims reads what a build currently argues about what it ships.
func (s *Store) OpenClaims(ctx context.Context, targetID int64) ([]Claim, error) {
	return openClaims(ctx, s.db, targetID)
}

func openClaims(ctx context.Context, db bun.IDB, targetID int64) ([]Claim, error) {
	// Ordered so that reading them twice gives the same answer. Where two
	// claims cover one finding and neither is the precise sort, which one is
	// recorded should not depend on what a map felt like doing.
	var rows []Claim
	err := db.NewSelect().Model(&rows).
		Where("target_id = ?", targetID).Where("closed_scan_id IS NULL").
		Order("id").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read what this build argues: %w", err)
	}
	return rows, nil
}
