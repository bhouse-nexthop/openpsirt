package httpapi

import (
	"errors"

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
func ambiguousOrMissing(err error) error {
	if errors.Is(err, graph.ErrAmbiguous) {
		return huma.Error409Conflict(
			"this build ships that name at more than one version — say which with ?version=")
	}
	return noSuchFinding()
}
