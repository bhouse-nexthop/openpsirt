package sbom

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// The format we read. A document that does not say it is this is refused
// rather than attempted, because a reader that guesses will eventually guess
// wrong on a file that looks close enough.
const formatName = "CycloneDX"

// Only the first major version exists, and every field read here has been in
// it throughout. A second major version would move things, so it is refused
// rather than read on the assumption that it did not.
const supportedMajor = "1"

// ReadHeader reads what a document says about itself and stops.
//
// The contents are skipped rather than parsed, so this stays cheap on a file
// that is about to be refused. It is not free — the interesting fields are not
// guaranteed to come first, and some producers sort their keys — but skipping
// values costs a walk rather than a structure per component.
func ReadHeader(r io.Reader, lim Limits) (Header, error) {
	c := newReader(r, lim, true)
	if err := c.read(); err != nil {
		return Header{}, err
	}
	return c.doc.Header, nil
}

// Read reads a whole document.
//
// Nothing partial is returned. A half-read inventory is indistinguishable from
// a product that shrank, and acting on one would close findings that are still
// somebody's problem.
func Read(r io.Reader, lim Limits) (*Document, error) {
	c := newReader(r, lim, false)
	if err := c.read(); err != nil {
		return nil, err
	}
	return c.finish()
}

// refEdge is one declared dependency, still named by the identifiers the file
// used. Those identifiers never leave this package: nothing guarantees a
// producer keeps them stable between builds, so they are good for joining a
// document to itself and for nothing else.
type refEdge struct{ parent, child string }

type reader struct {
	b          *bounded
	lim        Limits
	headerOnly bool

	format string
	spec   string

	doc Document
	// byRef resolves a document's own identifiers to the components they
	// named, for the length of the read.
	byRef map[string]graph.Described
	// described is every component in document order, one entry per identity.
	described []graph.Described
	seen      map[string]bool
	edges     []refEdge
	// contained is the structure a producer declared by nesting one component
	// inside another. It resolves without the document's identifiers, since a
	// nested component often carries none.
	contained []graph.Dependency
}

func newReader(r io.Reader, lim Limits, headerOnly bool) *reader {
	lim = lim.orDefault()
	return &reader{
		b:          newBounded(&capped{r: r, left: lim.MaxBytes}, lim.MaxDepth),
		lim:        lim,
		headerOnly: headerOnly,
		byRef:      map[string]graph.Described{},
		seen:       map[string]bool{},
	}
}

// read walks the document once.
func (c *reader) read() error {
	err := c.b.object(func(key string) error {
		switch key {
		case "bomFormat":
			if err := c.into(&c.format); err != nil {
				return err
			}
			return c.checkFormatName()
		case "specVersion":
			if err := c.into(&c.spec); err != nil {
				return err
			}
			return c.checkFormatVersion()
		case "serialNumber":
			return c.into(&c.doc.Serial)
		case "metadata":
			return c.metadata()
		case "components":
			if c.headerOnly {
				return c.b.skip()
			}
			_, err := c.componentArray()
			return err
		case "dependencies":
			if c.headerOnly {
				return c.b.skip()
			}
			return c.dependencies()
		default:
			return c.b.skip()
		}
	})
	if err != nil {
		return fmt.Errorf("reading scan file: %w", err)
	}
	return c.checkFormat()
}

// checkFormatName refuses a document that says it is something else.
//
// Checked where it is read rather than at the end, so a file that was never
// going to be read is dropped before the rest of it is walked.
func (c *reader) checkFormatName() error {
	if !strings.EqualFold(c.format, formatName) {
		return fmt.Errorf("scan file is not %s: it says %q", formatName, trim(c.format))
	}
	return nil
}

// checkFormatVersion refuses a version this reader was not written against.
func (c *reader) checkFormatVersion() error {
	if major, _, _ := strings.Cut(c.spec, "."); major != supportedMajor {
		return fmt.Errorf("%s version %q is not one this reads", formatName, trim(c.spec))
	}
	return nil
}

// checkFormat refuses anything this reader was not written against, including
// a document that never said what it was.
//
// What it does not check is that the document named a component of its own.
// The format does not require one, and the scan was filed against something
// that says what it is about.
func (c *reader) checkFormat() error {
	if err := c.checkFormatName(); err != nil {
		return err
	}
	if err := c.checkFormatVersion(); err != nil {
		return err
	}
	return nil
}

// metadata reads what the document says about itself.
func (c *reader) metadata() error {
	return c.b.object(func(key string) error {
		switch key {
		case "timestamp":
			raw, err := c.b.str()
			if err != nil {
				return err
			}
			if raw == "" {
				return nil
			}
			built, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return fmt.Errorf("build time %q is not a time: %w", trim(raw), err)
			}
			c.doc.BuiltAt = built.UTC()
			return nil
		case "component":
			return c.rootComponent()
		default:
			return c.b.skip()
		}
	})
}

// rootComponent reads the component the document is about.
//
// It is stored like any other and marked, which is what lets anything walking
// upwards stop at it. Its version changes on every build and its name differs
// per variant, so it stays out of identity and out of expiry.
func (c *reader) rootComponent() error {
	described, ref, nested, err := c.component()
	if err != nil {
		return err
	}
	c.doc.Root = described
	c.doc.RootDeclared = true
	if c.headerOnly {
		return nil
	}
	if err := c.bind(ref, described); err != nil {
		return err
	}
	for _, member := range nested {
		if err := c.contain(described, member); err != nil {
			return err
		}
	}
	return nil
}

// bind records what one of the document's own identifiers refers to.
func (c *reader) bind(ref string, described graph.Described) error {
	if ref == "" {
		return nil
	}
	if _, clash := c.byRef[ref]; clash {
		return fmt.Errorf("two components share the identifier %q, so every edge naming it is ambiguous", trim(ref))
	}
	c.byRef[ref] = described
	return nil
}

// componentArray reads an array of components and returns the ones directly in
// it. Anything nested deeper has already been recorded by the time it returns.
func (c *reader) componentArray() ([]graph.Described, error) {
	var members []graph.Described
	err := c.b.array(func() error {
		described, ref, nested, err := c.component()
		if err != nil {
			return err
		}
		if err := c.add(described); err != nil {
			return err
		}
		if err := c.bind(ref, described); err != nil {
			return err
		}
		for _, member := range nested {
			if err := c.contain(described, member); err != nil {
				return err
			}
		}
		members = append(members, described)
		return nil
	})
	return members, err
}

// component reads one component and any nested underneath it.
//
// Nested components are read here rather than gathered afterwards so that a
// document is only ever walked once, and so that the containment a producer
// declared by nesting survives.
func (c *reader) component() (graph.Described, string, []graph.Described, error) {
	var (
		described graph.Described
		ref       string
		nested    []graph.Described
		carried   []Suppression
	)
	err := c.b.object(func(key string) error {
		switch key {
		case "bom-ref":
			return c.into(&ref)
		case "name":
			return c.into(&described.Name)
		case "version":
			return c.into(&described.Version)
		case "purl":
			return c.into(&described.Purl)
		case "pedigree":
			return c.pedigree(&described, &carried)
		case "components":
			members, err := c.componentArray()
			if err != nil {
				return err
			}
			nested = append(nested, members...)
			return nil
		default:
			return c.b.skip()
		}
	})
	if err != nil {
		return graph.Described{}, "", nil, err
	}
	if err := described.Valid(); err != nil {
		return graph.Described{}, "", nil, fmt.Errorf("%w, so it cannot be tracked", err)
	}
	if strings.TrimSpace(described.Version) == "" {
		c.doc.Unversioned++
	}
	// A claim the pedigree carries is about the component it was read from,
	// which is only fully known now: key order is the producer's business, so
	// the patches may well have been read before the name they belong to.
	for _, claim := range carried {
		claim.Targets = []Target{{Purl: described.Purl, Name: described.Name}}
		c.doc.Suppressions = append(c.doc.Suppressions, claim)
	}
	return described, ref, nested, nil
}

// pedigree reads where a component came from, and what its patches say they
// fix.
//
// A shipped fork carries a version string of its own while the vulnerability
// lives on the version it was forked from, so dropping the ancestor makes
// findings unexplainable. The first ancestor is the one the producer forked
// from; anything further back is history rather than identity.
func (c *reader) pedigree(described *graph.Described, carried *[]Suppression) error {
	return c.b.object(func(key string) error {
		switch key {
		case "ancestors":
			return c.ancestor(described)
		case "patches":
			return c.patches(carried)
		default:
			return c.b.skip()
		}
	})
}

// ancestor reads what a component was forked from.
func (c *reader) ancestor(described *graph.Described) error {
	first := true
	return c.b.array(func() error {
		if !first {
			return c.b.skip()
		}
		first = false
		return c.b.object(func(field string) error {
			switch field {
			case "name":
				return c.into(&described.UpstreamName)
			case "version":
				return c.into(&described.UpstreamVersion)
			default:
				return c.b.skip()
			}
		})
	})
}

// patches reads what a component's carried patches say they resolve.
//
// This is the build's judgement about its own patches, arriving attached to
// the component it is about rather than in a separate document that has to be
// matched back to one. A patch only claims a vulnerability where it says so —
// in its own name, or in a header declaring what it fixes — so what is read
// here is a claim rather than a mention.
func (c *reader) patches(carried *[]Suppression) error {
	return c.b.array(func() error {
		var (
			diff    string
			claimed []Suppression
		)
		err := c.b.object(func(key string) error {
			switch key {
			case "diff":
				return c.b.object(func(field string) error {
					if field != "url" {
						return c.b.skip()
					}
					return c.into(&diff)
				})
			case "resolves":
				return c.b.array(func() error {
					claim, err := c.resolved()
					if err != nil {
						return err
					}
					if claim.Vulnerability != "" {
						claimed = append(claimed, claim)
					}
					return nil
				})
			default:
				return c.b.skip()
			}
		})
		if err != nil {
			return err
		}
		for _, claim := range claimed {
			claim.Statement = "resolved by a patch the build carries: " + trim(diff)
			*carried = append(*carried, claim)
		}
		return nil
	})
}

// resolved reads one thing a patch says it fixes.
//
// A patch may resolve a defect or an improvement as readily as a
// vulnerability, and only the last of those is a claim about security.
func (c *reader) resolved() (Suppression, error) {
	var (
		kind  string
		claim = Suppression{Status: AlreadyFixed, Origin: FromPedigree}
	)
	err := c.b.object(func(key string) error {
		switch key {
		case "type":
			return c.into(&kind)
		case "id":
			return c.into(&claim.Vulnerability)
		default:
			return c.b.skip()
		}
	})
	if err != nil {
		return Suppression{}, err
	}
	if kind != "security" {
		return Suppression{}, nil
	}
	return claim, nil
}

// dependencies reads the declared edges.
func (c *reader) dependencies() error {
	return c.b.array(func() error {
		var (
			ref      string
			children []string
		)
		err := c.b.object(func(key string) error {
			switch key {
			case "ref":
				return c.into(&ref)
			case "dependsOn":
				return c.b.array(func() error {
					child, err := c.b.str()
					if err != nil {
						return err
					}
					children = append(children, child)
					return nil
				})
			default:
				return c.b.skip()
			}
		})
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := c.edge(ref, child); err != nil {
				return err
			}
		}
		return nil
	})
}

// add records a component, once per identity.
func (c *reader) add(described graph.Described) error {
	if c.headerOnly {
		return nil
	}
	identity := described.Identity()
	if c.seen[identity] {
		return nil
	}
	if len(c.described) >= c.lim.MaxComponents {
		return fmt.Errorf("scan file describes more than the %d component limit", c.lim.MaxComponents)
	}
	c.seen[identity] = true
	c.described = append(c.described, described)
	return nil
}

// declared counts the edges read so far, however they were stated.
func (c *reader) declared() int { return len(c.edges) + len(c.contained) }

// contain records one component holding another, which a producer declares by
// nesting rather than by naming an edge.
func (c *reader) contain(parent, child graph.Described) error {
	if c.declared() >= c.lim.MaxEdges {
		return fmt.Errorf("scan file declares more than the %d dependency limit", c.lim.MaxEdges)
	}
	c.contained = append(c.contained, graph.Dependency{Parent: parent, Child: child})
	return nil
}

// edge records a declared dependency.
func (c *reader) edge(parent, child string) error {
	if c.declared() >= c.lim.MaxEdges {
		return fmt.Errorf("scan file declares more than the %d dependency limit", c.lim.MaxEdges)
	}
	c.edges = append(c.edges, refEdge{parent: parent, child: child})
	return nil
}

// finish resolves the document's own identifiers into components.
func (c *reader) finish() (*Document, error) {
	rootIdentity := c.doc.Root.Identity()

	c.doc.Components = make([]graph.Described, 0, len(c.described))
	for _, described := range c.described {
		if described.Identity() == rootIdentity {
			continue
		}
		c.doc.Components = append(c.doc.Components, described)
	}

	declared := make([]graph.Dependency, 0, len(c.edges)+len(c.contained))
	for _, e := range c.edges {
		// An edge naming something the document never describes is dropped
		// rather than taken as a malformed file. Producers differ in how
		// completely they state a graph, and one unresolvable edge is not a
		// reason to reject every component in a document of tens of
		// thousands. Inventing the missing component is still not done: the
		// edge simply goes nowhere and is counted.
		parent, ok := c.byRef[e.parent]
		if !ok {
			c.doc.DanglingEdges++
			continue
		}
		child, ok := c.byRef[e.child]
		if !ok {
			c.doc.DanglingEdges++
			continue
		}
		declared = append(declared, graph.Dependency{Parent: parent, Child: child})
	}
	declared = append(declared, c.contained...)

	reached := map[string]bool{}
	pairs := map[[2]string]bool{}
	for _, dep := range declared {
		parent, child := dep.Parent.Identity(), dep.Child.Identity()
		if parent == child {
			// Two of a document's own identifiers turned out to describe the
			// same component. The producer could not have known — its
			// identifiers differ — and an edge from a component to itself
			// says nothing.
			c.doc.SelfReferences++
			continue
		}
		pair := [2]string{parent, child}
		if pairs[pair] {
			continue
		}
		pairs[pair] = true
		reached[child] = true
		c.doc.Dependencies = append(c.doc.Dependencies, dep)
	}

	for _, described := range c.doc.Components {
		if !reached[described.Identity()] {
			c.doc.Unrooted++
		}
	}
	return &c.doc, nil
}

// into reads one string into dst.
func (c *reader) into(dst *string) error {
	value, err := c.b.str()
	if err != nil {
		return err
	}
	*dst = value
	return nil
}

// trim bounds what a message quotes back.
//
// Everything in a scan file was written by somebody else, and an error is one
// of the few places it reaches a person. A name the length of the file would
// make a log unreadable and a response unbounded.
func trim(s string) string {
	const most = 120
	if utf8.RuneCountInString(s) <= most {
		return s
	}
	return string([]rune(s)[:most]) + "…"
}
