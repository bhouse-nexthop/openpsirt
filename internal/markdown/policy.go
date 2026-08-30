package markdown

import (
	"fmt"
	"regexp"
	"strings"
)

// Schemes are the only ones a link may use.
//
// `javascript:` in a link is the oldest attack there is, and a `data:` URI
// lets a link become a page we appear to have served. Everything else is
// dropped rather than argued about, autolinked text included.
var Schemes = map[string]bool{"http": true, "https": true, "mailto": true}

// Languages are the fenced-block tags that may reach a class attribute.
//
// The tag is somebody's input. Three backticks followed by attacker-chosen
// text landing in markup is small and real, and it is the only
// highlighting-related hole left once the highlighter runs over already
// sanitized markup rather than before it.
//
// An unknown language renders as a plain block rather than an error: refusing
// a justification because somebody wrote a language nobody listed would make
// the tool argue with people about syntax highlighting.
var Languages = map[string]bool{
	"bash": true, "c": true, "cpp": true, "diff": true, "dockerfile": true,
	"go": true, "hcl": true, "ini": true, "java": true, "javascript": true,
	"json": true, "makefile": true, "markdown": true, "nginx": true,
	"none": true, "patch": true, "perl": true, "php": true, "python": true,
	"ruby": true, "rust": true, "shell": true, "sql": true, "text": true,
	"toml": true, "typescript": true, "xml": true, "yaml": true,
}

var (
	// A markdown image, which is the whole category: rendered markdown fetches
	// nothing from anywhere.
	image = regexp.MustCompile(`!\[[^\]]*\]\(([^)]*)\)`)
	// A markdown link.
	link = regexp.MustCompile(`\[[^\]]*\]\(([^)]*)\)`)
	// A bare address that a renderer would turn into a link. Written as
	// "scheme://" rather than "word colon", because the second matches
	// `parser.go:112` in a stack trace and `TODO: check this` in a sentence —
	// and a policy that argues with a stack trace is one people route around.
	autolink = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*)://[^\s<>]+`)
	// The schemes that do something rather than go somewhere. These carry no
	// "//" so the pattern above does not see them, and they are the reason
	// this check exists at all.
	executable = regexp.MustCompile(`(?i)\b(javascript|data|vbscript|file)\s*:`)
	// An HTML tag. Raw HTML is off at the parser, so this is not what stops
	// it — it is here so somebody who wrote a tag is told why it will not
	// appear, rather than watching it vanish.
	htmlTag = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
)

// faultsIn reports what is wrong with one line.
func faultsIn(number int, line string) []Fault {
	var faults []Fault

	// Images first, because every image is refused and a link that happens to
	// look like one should be reported as the image it is.
	for _, match := range image.FindAllStringSubmatch(line, -1) {
		faults = append(faults, Fault{
			Line: number, Offending: match[0],
			Reason: "images are not shown, because a rendered image is fetched by the browser of " +
				"everybody who reads this — from inside the network, telling whoever wrote it who " +
				"is looking and when. Attach the file instead",
		})
	}

	for _, match := range link.FindAllStringSubmatch(line, -1) {
		if strings.HasPrefix(match[0], "!") {
			continue // already reported as an image
		}
		if scheme, ok := schemeOf(match[1]); !ok {
			faults = append(faults, Fault{
				Line: number, Offending: match[1],
				Reason: fmt.Sprintf("a link may use http, https or mailto, and this uses %q", scheme),
			})
		}
	}

	for _, match := range autolink.FindAllStringSubmatch(line, -1) {
		// Inside a link's parentheses it has already been judged.
		if strings.Contains(line, "]("+match[0]) {
			continue
		}
		if !Schemes[strings.ToLower(match[1])] {
			faults = append(faults, Fault{
				Line: number, Offending: match[0],
				Reason: fmt.Sprintf("a link may use http, https or mailto, and this uses %q", match[1]),
			})
		}
	}

	for _, match := range executable.FindAllStringSubmatch(line, -1) {
		// Reported wherever it appears rather than only inside a link. Such a
		// scheme has no honest use in a justification, and the shapes that get
		// one past a check for links are exactly the interesting ones.
		faults = append(faults, Fault{
			Line: number, Offending: match[0],
			Reason: fmt.Sprintf("%q does not go anywhere — it does something, which a justification may not",
				strings.TrimSpace(match[1])),
		})
	}

	if found := htmlTag.FindString(line); found != "" {
		faults = append(faults, Fault{
			Line: number, Offending: found,
			Reason: "markup is not interpreted here and will be shown as written. " +
				"Use markdown, or a fenced block if you meant to show the tag itself",
		})
	}

	return faults
}

// schemeOf reads a link's scheme, and whether it is one of ours.
//
// A link with no scheme at all is relative, which is allowed and goes nowhere
// outside. A fragment or a query is the same.
func schemeOf(target string) (string, bool) {
	target = strings.TrimSpace(target)
	// A title may follow the address: [text](https://example.com "title").
	if space := strings.IndexAny(target, " \t"); space > 0 {
		target = target[:space]
	}
	if target == "" {
		return "", true
	}
	if strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") ||
		strings.HasPrefix(target, "?") || strings.HasPrefix(target, ".") {
		return "", true
	}

	scheme, _, found := strings.Cut(target, ":")
	if !found {
		return "", true
	}
	// A colon inside a path rather than a scheme separator.
	if strings.ContainsAny(scheme, "/?# ") {
		return "", true
	}
	lowered := strings.ToLower(scheme)
	return lowered, Schemes[lowered]
}
