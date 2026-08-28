// Package sbom reads the inventory a build produced.
//
// A build sends what it shipped: every component, and the edges between them
// it was able to derive. The vulnerability data is not in it — that is
// produced here, against a database that moves daily, which is what lets a
// year-old release be re-examined without rebuilding it.
//
// Producers differ in far more than the format they emit, so reading is a
// seam: whatever a producer sends is read into the shapes below, and nothing
// downstream knows which producer it came from. The shapes are deliberately
// the ones the graph is stored in — a component as a scan describes it, and an
// edge between two of them — rather than a parallel model that would have to
// be kept in step with it.
package sbom

import (
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// Limits bound what a document may contain.
//
// A scan file is somebody else's output, arriving over a link we do not
// control, and a broken or hostile one must fail rather than exhaust the
// process. The defaults sit well above the largest real producer rather than
// near it: refusing a legitimate file is its own failure, and the ceiling only
// has to be low enough to protect the process.
type Limits struct {
	// MaxBytes is how large a document may be.
	MaxBytes int64
	// MaxComponents is how many components it may describe.
	MaxComponents int
	// MaxEdges is how many dependency edges it may declare. Component count
	// alone does not bound this — a document with a thousand components can
	// declare a million edges between them.
	MaxEdges int
	// MaxDepth is how deeply it may nest.
	MaxDepth int
}

// DefaultLimits are the bounds a reader uses when given none.
//
// The largest producer we have emits tens of megabytes and tens of thousands
// of components; these are several times that.
func DefaultLimits() Limits {
	return Limits{
		MaxBytes:      256 << 20,
		MaxComponents: 250_000,
		MaxEdges:      2_000_000,
		MaxDepth:      64,
	}
}

// orDefault fills in anything left unset, so a caller that wants one bound
// changed does not have to restate the rest.
func (l Limits) orDefault() Limits {
	d := DefaultLimits()
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxComponents <= 0 {
		l.MaxComponents = d.MaxComponents
	}
	if l.MaxEdges <= 0 {
		l.MaxEdges = d.MaxEdges
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = d.MaxDepth
	}
	return l
}

// Header is what a document says about itself, separately from its contents.
//
// It is read on its own because the questions asked of an arriving scan —
// whether it is newer than what we hold, whether we have taken it already,
// whether its build time is believable — are all answered from here. Reading
// the contents to answer them would mean parsing files we are about to refuse.
type Header struct {
	// Serial is the identity the document carries for itself. It is what
	// joins a vulnerability report to the inventory it was produced from,
	// since filenames and upload order say nothing once documents have been
	// copied away from the build tree.
	Serial string
	// BuiltAt is when the producer says the document was made. This is what
	// orders scans against each other.
	BuiltAt time.Time
	// Root is the component the document is about — the product itself.
	Root graph.Described
}

// Document is one build's inventory.
type Document struct {
	Header
	// Components is everything the document describes, deduplicated by
	// identity. The root is not repeated here.
	Components []graph.Described
	// Dependencies is every edge, by the components it joins rather than by
	// the identifiers the file used for them.
	Dependencies []graph.Dependency
	// Unrooted counts components no edge leads to. An incomplete graph is
	// ordinary: a producer emits the edges it can derive and records what it
	// could not, and inventing the rest would report dependencies nobody
	// declared. The count is kept because a sudden change in it says the
	// producer's derivation changed, which is worth seeing.
	Unrooted int
	// SelfReferences counts edges dropped for having the same component at
	// both ends. Producers do not emit those deliberately; they appear when
	// two of a document's own identifiers turn out to describe the same
	// component, which is a thing content-derived identity can discover and
	// the producer cannot.
	SelfReferences int
}

// Snapshot returns the graph the document describes.
func (d *Document) Snapshot() graph.Snapshot {
	return graph.Snapshot{
		Root:         d.Root,
		Components:   d.Components,
		Dependencies: d.Dependencies,
	}
}
