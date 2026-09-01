package currency

import (
	"net/url"
	"strings"
	"time"
)

// Asked reads the ecosystem and the name to ask it, out of a package
// identifier.
//
// Only the identifier is read. A component's own name is what a producer chose
// to call it, and the indexes here are keyed on what the ecosystem calls it —
// which for a Go module is a path and for a scoped npm package is two segments
// that have to arrive as one name.
//
// The second return says whether there is anything to ask at all. A component
// with no identifier, or one in an ecosystem with no index we ask, is not a
// failure: a distribution package, a private module and a vendored fork all
// look like this.
func Asked(purl string) (ecosystem, name string, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(purl), "pkg:")
	if !found {
		return "", "", false
	}
	// Everything after the path is somebody else's business: qualifiers first,
	// then a subpath, which may itself contain the separators below.
	rest, _, _ = strings.Cut(rest, "#")
	rest, _, _ = strings.Cut(rest, "?")

	ecosystem, path, found := strings.Cut(rest, "/")
	if !found || path == "" {
		return "", "", false
	}
	ecosystem = strings.ToLower(ecosystem)

	// The version is cut from the right: a namespace may contain no "@", but
	// an npm scope begins with one, so the last is the separator.
	if at := strings.LastIndex(path, "@"); at > 0 {
		path = path[:at]
	}

	// Each segment is decoded on its own, because the separator between them
	// is structure rather than content — a name containing an escaped "/" is
	// one segment, and decoding the whole path first would make it two.
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if unescaped, err := url.PathUnescape(segment); err == nil {
			segments[i] = unescaped
		}
	}
	name = strings.Join(segments, "/")
	if name == "" {
		return "", "", false
	}
	return ecosystem, name, true
}

// Identified reads the year an issue was identified in, out of its identifier.
//
// Zero where the identifier does not name one. CVE and several others begin
// with a year; GHSA and a handful more do not, and guessing for those would
// invent the very fact this is here to supply.
func Identified(identifier string) int {
	for _, part := range strings.Split(identifier, "-") {
		if len(part) != 4 {
			continue
		}
		year := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				year = 0
				break
			}
			year = year*10 + int(r-'0')
		}
		// A sanity range rather than any four digits: an identifier can carry
		// a four-digit sequence number that is not a year, and reading one as
		// 3521 would make every release look ancient.
		if year >= 1999 && year <= 2100 {
			return year
		}
	}
	return 0
}

// NothingSince reports whether upstream's newest release predates the year the
// issue was identified in.
//
// The reason there is no fix, reached by comparing two dates rather than by
// rating anybody's project (ING-41). If the newest thing upstream has shipped
// came out before the year the flaw was named, then upstream has shipped
// nothing since it became known — which is arithmetic, and says nothing about
// whether the project is abandoned, busy, or disagrees that it is a flaw.
//
// A disclosure date would be more precise and we do not have one (REJ-11).
// The year is enough because the gap this is worth reporting is measured in
// years: an issue named in 2021 against a component last released in 2019 is
// the case, and one named in January against a release the previous December
// is not.
func NothingSince(identifier string, released time.Time) bool {
	year := Identified(identifier)
	if year == 0 || released.IsZero() {
		return false
	}
	return released.UTC().Year() < year
}
