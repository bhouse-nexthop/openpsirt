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

func noSuchAssessment() error {
	return huma.Error404NotFound("no assessment is recorded there")
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
// ambiguousAmong offers the ways a name could be meant, having narrowed them
// to the ones that answer the question being asked.
func ambiguousAmong(name string, choices []graph.Choice) error {
	detail := make([]error, 0, len(choices))
	for _, choice := range choices {
		detail = append(detail, &huma.ErrorDetail{
			// "carrying" rather than "component", because these have been
			// narrowed to the ones this issue is actually open at. The screen
			// says different things about the two, and saying the wrong one is
			// telling somebody an issue affects a version it does not.
			Location: "carrying", Message: choice.Version,
			Value: map[string]string{"ecosystem": choice.Ecosystem},
		})
	}
	return huma.Error409Conflict(fmt.Sprintf(
		"this build ships %q at more than one version, and this issue is open at %d of "+
			"them — say which one with ?version=", name, len(choices)), detail...)
}

// ambiguousOrMissing answers a component lookup that could not settle on one.
//
// A name matching several is a different answer from a name matching none: the
// first is something the caller can fix by saying which one, and the second is
// not. Telling them apart discloses nothing — whoever is asking has already
// been authorized to read this build.
func ambiguousOrMissing(err error) error {
	// The choices as structured detail rather than a sentence listing them. A
	// real image ships one library at fifteen versions, and fifteen of them in
	// prose is a paragraph nobody reads — as a list the screen can offer each
	// as a link, which is the thing the reader actually wants. Naming them
	// discloses nothing further: the versions in a build are already readable
	// by anyone who can read the build.
	//
	// Each choice carries its ecosystem, because a version alone does not
	// always resolve one: 13 names in a real image are held at one version by
	// two components, a source repository and the package built from it. Left
	// as versions alone, the refusal offered a choice that led straight back
	// to the same refusal.
	var several *graph.Ambiguous
	if errors.As(err, &several) {
		return severalComponents(several, "?version= and, where two share a version, &ecosystem=")
	}
	return noSuchFinding()
}

// severalComponents offers every way a name could be meant, and says how to
// pick one.
//
// How to say which differs by where the name arrived — a query parameter for a
// lookup, a body field for something being recorded — so the caller supplies
// that sentence and the list is built once.
func severalComponents(several *graph.Ambiguous, sayWith string) error {
	detail := make([]error, 0, len(several.Choices))
	for _, choice := range several.Choices {
		detail = append(detail, &huma.ErrorDetail{
			// Every component of that name, *not* narrowed to an issue —
			// which is why the location differs from the narrowed list above.
			// Some of these may not carry it at all.
			Location: "component", Message: choice.Version,
			Value: map[string]string{"ecosystem": choice.Ecosystem},
		})
	}
	return huma.Error409Conflict(fmt.Sprintf(
		"this build ships %q as %d different components — say which one with %s",
		several.Name, len(several.Choices), sayWith), detail...)
}
