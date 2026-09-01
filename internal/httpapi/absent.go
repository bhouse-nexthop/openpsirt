package httpapi

import (
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// What to say when a name reaches nothing.
//
// One place, because there were several spellings of the same sentence and two
// that described the wrong thing entirely — a missing person and a missing
// credential both answered "not declared", which names neither and reads as
// though the request were about a product.
//
// These are all 404, including for things somebody may not reach. A product
// somebody holds nothing on is invisible rather than merely unreadable, and
// telling "you may not see that" apart from "that does not exist" hands
// somebody holding one product the name of every other, by guessing and
// watching which guess answers differently.
func noSuchProduct() error {
	return huma.Error404NotFound("no product is declared by that name")
}

func noSuchPerson() error {
	return huma.Error404NotFound("nobody here is called that")
}

func noSuchKey() error {
	return huma.Error404NotFound("no credential is recorded under that name")
}

func noSuchDecision() error {
	return huma.Error404NotFound("no decision is recorded there")
}

func noSuchFinding() error {
	return huma.Error404NotFound("no open finding is recorded there")
}

func noSuchIssue() error {
	return huma.Error404NotFound("no issue is known by that name")
}

func nothingScannedThere() error {
	return huma.Error404NotFound("nothing has been scanned there")
}

// ambiguousOrMissing answers a component lookup that could not settle on one.
//
// A name matching several is a different answer from a name matching none: the
// first is something the caller can fix by saying which version, and the
// second is not. Telling them apart discloses nothing — whoever is asking has
// already been authorized to read this build.
// ambiguousAmong offers a named set of versions rather than every version of
// the name.
func ambiguousAmong(name string, versions []string) error {
	if len(versions) == 1 {
		// One choice is not a choice. Whoever followed the link wanted this.
		return huma.Error409Conflict(fmt.Sprintf(
			"this build ships %q at more than one version, and this issue is open at "+
				"one of them — add ?version=%s", name, versions[0]),
			&huma.ErrorDetail{Location: "version", Message: versions[0]})
	}
	detail := make([]error, 0, len(versions))
	for _, version := range versions {
		detail = append(detail, &huma.ErrorDetail{Location: "version", Message: version})
	}
	return huma.Error409Conflict(fmt.Sprintf(
		"this build ships %q at more than one version, and this issue is open at %d of "+
			"them — say which one with ?version=", name, len(versions)), detail...)
}

func ambiguousOrMissing(err error) error {
	// The versions, not only the fact. "Say which version" is not answerable
	// by somebody who does not know what the choices are, and this is usually
	// reached by following a link — so whoever hit it has nothing else to go
	// on. Naming them discloses nothing further: the versions in a build are
	// already readable by anyone who can read the build.
	var several *graph.Ambiguous
	if errors.As(err, &several) {
		// The versions as structured detail rather than a sentence listing
		// them. A real image ships one library at fifteen versions, and
		// fifteen of them in prose is a paragraph nobody reads — as a list the
		// screen can offer each as a link, which is the thing the reader
		// actually wants. Naming them discloses nothing further: the versions
		// in a build are already readable by anyone who can read the build.
		detail := make([]error, 0, len(several.Versions))
		for _, version := range several.Versions {
			detail = append(detail, &huma.ErrorDetail{
				Location: "version", Message: version,
			})
		}
		return huma.Error409Conflict(fmt.Sprintf(
			"this build ships %q at %d versions — say which one with ?version=",
			several.Name, len(several.Versions)), detail...)
	}
	if errors.Is(err, graph.ErrAmbiguous) {
		return huma.Error409Conflict(
			"this build ships that name at more than one version — say which with ?version=")
	}
	return noSuchFinding()
}
