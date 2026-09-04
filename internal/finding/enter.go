package finding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// Entering is a flaw somebody is recording in what this deployment ships.
//
// Not something a scanner found. A scanner reports a known issue in a
// component somebody else wrote; this is a vulnerability in our own product,
// usually before anybody outside knows about it — which is why it starts
// undisclosed and why the whole disclosure apparatus hangs off it.
type Entering struct {
	// TargetID is the build it is in. A flaw is in something we shipped, and
	// which release is the first question anybody asks.
	TargetID int64
	// Component names what in the build carries it, as the build calls it.
	// Empty is the build itself, which is the honest answer where the flaw is
	// in how the pieces are put together rather than in one of them.
	Component string
	// Summary is what the flaw is, in the words of whoever found it. It is
	// what a triager reads first and often all they read.
	Summary string
	// Severity is how bad it is judged to be, in the same words a report uses,
	// so that everything ranking and clocking findings treats it the same way.
	Severity string
	// Disclosed says this is already public. The default is that it is not:
	// somebody recording a flaw in their own product before it is announced is
	// the case this exists for, and defaulting the other way makes the
	// dangerous mistake the quiet one.
	Disclosed bool
}

// rated is the severity words somebody may record. The same set a report may
// carry, so that a finding a person entered ranks and expires beside the ones a
// scanner found rather than in a scheme of its own.
var rated = map[string]bool{
	"critical": true, "high": true, "medium": true,
	"low": true, "negligible": true, "none": true,
}

// ErrNoSuchComponent says the build holds nothing by that name.
var ErrNoSuchComponent = errors.New("this build holds nothing by that name")

// Enter records a flaw in what a build ships, and returns the finding and the
// identifier it was filed under.
//
// **It is filed under an identifier this deployment mints**, because there is
// nothing else to file it under: a flaw nobody has published has no CVE, and
// waiting for one would mean the record of what we knew starts after the work
// does. The identifier is the product's own name, the year, and a number —
// `SONIC-2026-0001` — which is the shape a vendor advisory already takes.
// When a CVE is assigned later it is recorded as another name for the same
// issue, and the issue is then filed under the CVE; nothing about the finding,
// the decisions or the approvals moves, because they are keyed on the issue
// rather than on what it is called (MDL-19).
//
// **It opens with no run**, and everything that asks when a finding opened
// reads the row rather than the run. A scan will not close it either: a run is
// the authority on what it found, and it found none of this.
func (s *Store) Enter(ctx context.Context, subject access.Subject, in Entering) (*Finding, string, error) {
	productID, err := productOf(ctx, s.db, in.TargetID)
	if err != nil {
		return nil, "", err
	}

	// Recording a flaw in our own product is triage work on a finding nobody
	// has disclosed, so it asks for the right that names that. Public triage
	// is not enough: somebody who may argue about known issues in shipped
	// components has not been given the undisclosed ones.
	visibility := access.Private
	if in.Disclosed {
		visibility = access.Public
	}
	// The same composition every other triage decision uses: an undisclosed
	// finding asks for the private right, and a public one is covered by
	// either. Somebody trusted with what nobody has announced is certainly
	// trusted with what everybody has, and spelling that a second way here
	// would be a second rule to keep in step with the first.
	if !mayRecord(subject, productID, visibility) {
		return nil, "", access.Denied(
			fmt.Sprintf("record a finding in product %d", productID))
	}
	if subject.ID == 0 {
		return nil, "", access.Denied("record a finding without being anybody")
	}
	if strings.TrimSpace(in.Summary) == "" {
		return nil, "", fmt.Errorf("a recorded finding has to say what the flaw is")
	}
	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if !rated[severity] {
		// Checked against the words rather than folded through Band, which is
		// what a *report* goes through. Band answers "medium" for anything it
		// does not recognize, deliberately: a scanner that rated nothing is
		// silent, and silence is not a claim that something is mild. A person
		// typing "urgent" is not silent — they are wrong, and folding it would
		// replace their judgment with one nobody made.
		return nil, "", fmt.Errorf("%q is not a severity", in.Severity)
	}

	componentID, componentName, err := s.carrying(ctx, in.TargetID, in.Component)
	if err != nil {
		return nil, "", err
	}

	product, err := productNameOf(ctx, s.db, productID)
	if err != nil {
		return nil, "", err
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	var row *Finding
	var identifier string
	err = database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// Minted inside the transaction. The number is one past the highest
		// this product has issued this year, and reading it before the
		// transaction began would describe a world another writer has since
		// moved (DAT-31).
		identifier, err = mint(ctx, tx, product, now.Year())
		if err != nil {
			return err
		}
		interned, err := NewVulnerabilities(tx).Intern(ctx, []Named{{
			Identifier:  identifier,
			Severity:    severity,
			Description: in.Summary,
		}})
		if err != nil {
			return err
		}
		vulnerabilityID := interned[identifier]

		windows, err := LoadWindows(ctx, tx)
		if err != nil {
			return err
		}
		embargo, err := setting.NewStore(tx).Duration(ctx,
			setting.DiscloseAfter, setting.DefaultDiscloseAfter)
		if err != nil {
			return err
		}
		floor, err := FloorFor(ctx, tx, productID)
		if err != nil {
			return err
		}

		row = &Finding{
			TargetID: in.TargetID, Kind: Entered, Visibility: visibility,
			VulnerabilityID: vulnerabilityID,
			ComponentID:     componentID,
			PlaceIdentity:   PlaceIdentity(componentName, ""),
			LastChangedAt:   now,
			OpenedAt:        now,
		}
		// An embargo gets an end. A public finding gets none — it is already
		// disclosed, and a date on it would be a deadline for something that
		// has already happened (ACC-46).
		if visibility == access.Private {
			at := now.Add(embargo)
			row.DiscloseAt = &at
		}
		// Ranked and clocked exactly as a scanned finding is, from the same
		// signals. A finding that sorted or expired differently because a
		// person typed it would be a second policy nobody chose.
		ranked := Ranked{Shipped: true}
		if row.Urgency = int64(ranked.Rank()); floor.Admits(false, severity) {
			due := now.Add(windows.For(false, severity))
			row.DueAt = &due
		}
		if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
			return fmt.Errorf("record the finding: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return row, identifier, nil
}

// mayRecord reports whether a subject may record a finding of this visibility
// on this product.
//
// Recording is triage work — it creates the thing everything else argues about
// — so it asks the triage right rather than the right to read.
func mayRecord(subject access.Subject, productID int64, visibility access.Visibility) bool {
	if subject.Kind != access.Person {
		return false
	}
	if visibility == access.Private {
		return subject.Holds(access.PrivateTriage, productID)
	}
	return subject.Holds(access.PublicTriage, productID) ||
		subject.Holds(access.PrivateTriage, productID)
}

// carrying resolves what in the build holds the flaw, defaulting to the build
// itself.
func (s *Store) carrying(ctx context.Context, targetID int64, name string) (int64, string, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		var id int64
		err := s.db.NewSelect().
			TableExpr("graph_node AS n").
			Join("JOIN component AS c ON c.id = n.component_id").
			ColumnExpr("c.id").
			Where("n.target_id = ?", targetID).
			Where("n.closed_scan_id IS NULL").
			Where("c.name = ?", name).
			Limit(1).
			Scan(ctx, &id)
		if database.IsNoRows(err) {
			return 0, "", ErrNoSuchComponent
		}
		if err != nil {
			return 0, "", fmt.Errorf("look up what carries this: %w", err)
		}
		return id, name, nil
	}

	// The build itself. A flaw in how the pieces fit together belongs on the
	// thing that assembles them, and every build has a root — that is what the
	// inventory describes.
	var root struct {
		ID   int64  `bun:"id"`
		Name string `bun:"name"`
	}
	err := s.db.NewSelect().
		TableExpr("graph_node AS n").
		Join("JOIN component AS c ON c.id = n.component_id").
		ColumnExpr("c.id AS id").
		ColumnExpr("c.name AS name").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Where("n.is_root = ?", true).
		Limit(1).
		Scan(ctx, &root)
	if database.IsNoRows(err) {
		return 0, "", fmt.Errorf(
			"nothing has been scanned into this build, so there is nothing to record against")
	}
	if err != nil {
		return 0, "", fmt.Errorf("look up what this build is: %w", err)
	}
	return root.ID, root.Name, nil
}

// mint issues the next identifier this product has to give out this year.
//
// Shaped like a vendor advisory identifier because that is what it becomes:
// the product, the year, and a number that counts from one. Nothing infers the
// prefix from configuration — the product already has a name people type, and
// a second setting for it is a second thing to keep in step.
//
// The number is read and used inside one transaction, so two people recording
// at the same moment cannot be handed the same one: the second waits, reads
// the first's row, and gets the number after it.
func mint(ctx context.Context, tx bun.Tx, product string, year int) (string, error) {
	prefix := strings.ToUpper(strings.TrimSpace(product))
	if prefix == "" {
		return "", fmt.Errorf("a product with no name cannot issue an identifier")
	}
	like := fmt.Sprintf("%s-%d-%%", prefix, year)

	var issued []string
	err := tx.NewSelect().
		TableExpr("vulnerability AS v").
		ColumnExpr("v.identifier").
		Where("v.identifier LIKE ?", like).
		Scan(ctx, &issued)
	if err != nil {
		return "", fmt.Errorf("read what has been issued: %w", err)
	}

	highest := 0
	for _, identifier := range issued {
		var n int
		// Anything that does not parse is not one of ours and is skipped
		// rather than refused: the pattern can match an identifier somebody
		// else issued that happens to start the same way, and refusing then
		// would stop this product recording anything ever again.
		if _, err := fmt.Sscanf(identifier, prefix+"-%d-%d", &year, &n); err == nil && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("%s-%d-%04d", prefix, year, highest+1), nil
}

// productNameOf reads what a product is called, for the identifiers it issues.
func productNameOf(ctx context.Context, db bun.IDB, productID int64) (string, error) {
	var name string
	err := db.NewSelect().
		TableExpr("product AS p").
		ColumnExpr("p.name").
		Where("p.id = ?", productID).
		Scan(ctx, &name)
	if err != nil {
		return "", fmt.Errorf("look up what product %d is called: %w", productID, err)
	}
	return name, nil
}

// ComponentName reads what one component is called, for an answer that has to
// name what it just recorded against.
func (s *Store) ComponentName(ctx context.Context, componentID int64) (string, error) {
	var name string
	err := s.db.NewSelect().
		TableExpr("component AS c").
		ColumnExpr("c.name").
		Where("c.id = ?", componentID).
		Scan(ctx, &name)
	if err != nil {
		return "", fmt.Errorf("look up what component %d is called: %w", componentID, err)
	}
	return name, nil
}
