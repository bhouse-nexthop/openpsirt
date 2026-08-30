package markdown

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Schemes are the only ones a link may use.
//
// `javascript:` in a link is the oldest attack there is, and a `data:` address
// lets a link become a page we appear to have served. Everything else is
// refused rather than argued about, autolinked text included.
var Schemes = map[string]bool{"http": true, "https": true, "mailto": true}

// Languages are the fenced-block tags that may reach a class attribute.
//
// The tag is somebody's input. Three backticks followed by attacker-chosen
// text landing in markup is small and real, and it is the only
// highlighting-related hole left once the highlighter runs over already
// sanitized markup rather than before it.
//
// An unknown language keeps its block and loses the label rather than failing:
// refusing a justification because somebody wrote a language nobody listed
// would make the tool argue with people about syntax highlighting.
var Languages = map[string]bool{
	"bash": true, "c": true, "cpp": true, "diff": true, "dockerfile": true,
	"go": true, "hcl": true, "ini": true, "java": true, "javascript": true,
	"json": true, "makefile": true, "markdown": true, "nginx": true,
	"none": true, "patch": true, "perl": true, "php": true, "python": true,
	"ruby": true, "rust": true, "shell": true, "sql": true, "text": true,
	"toml": true, "typescript": true, "xml": true, "yaml": true,
}

// inspect reports what is wrong with submitted text.
//
// **The document is parsed and its structure examined, not scanned as lines.**
// The first version of this matched regular expressions against each line, and
// that is a different question from the one that matters: what the renderer
// will make of it. The two came apart in every direction —
//
//   - A destination is entity-decoded before it becomes a link, so
//     `&#106;avascript:` reads as nothing dangerous to a pattern and as
//     `javascript:` to the renderer.
//   - A reference definition puts the destination on a line of its own, far
//     from the text that uses it, so a pattern looking for `](…)` finds
//     nothing at all.
//   - A link may be written across several lines, which a line-by-line reader
//     cannot see as one thing.
//   - And `<https://example.com>` — the standard way to write a bare link —
//     looks exactly like a markup tag to a pattern, so honest text was refused.
//
// Asking the parser removes the whole class. What is checked here is what will
// be rendered, because it is the same parse.
func inspect(source string) []Fault {
	document := parser.Parser().Parse(text.NewReader([]byte(source)))
	lines := lineIndexFor(source)

	var faults []Fault
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.Image:
			destination := string(typed.Destination)
			faults = append(faults, Fault{
				Line:      lines.of(destination, lines.at(node)),
				Offending: destination,
				Reason: "images are not shown, because a rendered image is fetched by the browser of " +
					"everybody who reads this — from inside the network, telling whoever wrote it who " +
					"is looking and when. Attach the file instead",
			})
		case *ast.Link:
			destination := string(typed.Destination)
			if fault, bad := destinationFault(lines.of(destination, lines.at(node)), destination); bad {
				faults = append(faults, fault)
			}
		case *ast.AutoLink:
			destination := string(typed.URL([]byte(source)))
			if fault, bad := destinationFault(lines.of(destination, lines.at(node)), destination); bad {
				faults = append(faults, fault)
			}
		case *ast.RawHTML, *ast.HTMLBlock:
			faults = append(faults, Fault{
				Line: lines.at(node),
				Reason: "markup is not interpreted here and will be shown as written. " +
					"Use markdown, or a fenced block if you meant to show the tag itself",
			})
		}
		return ast.WalkContinue, nil
	})
	return faults
}

// destinationFault judges where a link goes.
func destinationFault(line int, destination string) (Fault, bool) {
	scheme, ok := schemeOf(destination)
	if ok {
		return Fault{}, false
	}
	return Fault{
		Line: line, Offending: destination,
		Reason: fmt.Sprintf("a link may use http, https or mailto, and this uses %q", scheme),
	}, true
}

// schemeOf reads where a destination goes, and whether it is somewhere a link
// may go.
//
// The destination arrives already decoded by the parser, which is the point:
// what is judged is where the link will actually point rather than how it was
// spelled.
//
// A destination with no scheme is relative. Those stay inside this deployment
// and are allowed — a link from one finding to another is ordinary.
func schemeOf(destination string) (string, bool) {
	// Decoded first, and this is the whole point. A destination is kept as it
	// was written and resolved when it is rendered, so `&#106;avascript:`
	// reads as harmless here and as `javascript:` in a browser. Judging the
	// spelling rather than the meaning is how a check gets walked past.
	destination = strings.TrimSpace(stdhtml.UnescapeString(destination))
	if destination == "" {
		return "", true
	}
	// Anything before a path separator, a query or a fragment is not a scheme.
	head := destination
	if cut := strings.IndexAny(head, "/?#"); cut >= 0 {
		head = head[:cut]
	}
	scheme, found := strings.CutSuffix(head, ":")
	if !found {
		// No colon before the first separator, so nothing is claiming to be a
		// scheme: this is relative.
		if !strings.Contains(head, ":") {
			return "", true
		}
		scheme, _, _ = strings.Cut(head, ":")
	}
	// Whitespace and control characters inside a scheme are how one gets past
	// a check that trusts the text: browsers strip them and act on what is
	// left.
	scheme = strings.Map(func(r rune) rune {
		if r <= ' ' {
			return -1
		}
		return r
	}, scheme)
	if scheme == "" {
		return "", true
	}

	lowered := strings.ToLower(scheme)
	return lowered, Schemes[lowered]
}

// lineIndex maps a position in the source to the line it is on.
type lineIndex struct {
	source []byte
	starts []int
}

func newLineIndex(source string) lineIndex {
	starts := []int{0}
	for offset, r := range []byte(source) {
		if r == '\n' {
			starts = append(starts, offset+1)
		}
	}
	return lineIndex{source: []byte(source), starts: starts}
}

// of returns the 1-indexed line the given text appears on.
//
// Used in preference to the enclosing block's position, because a paragraph
// may run for twenty lines and pointing at its first one sends somebody to the
// wrong place. What a person will do is search for the offending text, so this
// does the same.
func (l lineIndex) of(offending string, fallback int) int {
	if offending == "" {
		return fallback
	}
	at := bytes.Index(l.source, []byte(offending))
	if at < 0 {
		// The destination was decoded by the parser and does not appear
		// literally — an entity-encoded scheme, say. The block is then the
		// most precise honest answer.
		return fallback
	}
	for number := len(l.starts) - 1; number >= 0; number-- {
		if at >= l.starts[number] {
			return number + 1
		}
	}
	return fallback
}

// at returns the 1-indexed line a node begins on, or 0 where it cannot be
// placed. A fault that cannot say where it is still reports what is wrong.
func (l lineIndex) at(node ast.Node) int {
	// Only a block knows where it is. Asking an inline node is not merely
	// unanswerable — it panics — so the walk goes up to the block containing
	// it, which is the paragraph or list item a person would look at anyway.
	for node != nil && node.Type() != ast.TypeBlock && node.Type() != ast.TypeDocument {
		node = node.Parent()
	}
	if node == nil {
		return 0
	}

	offset := -1
	if lines := node.Lines(); lines != nil && lines.Len() > 0 {
		offset = lines.At(0).Start
	}
	if offset < 0 {
		return 0
	}
	for number := len(l.starts) - 1; number >= 0; number-- {
		if offset >= l.starts[number] {
			return number + 1
		}
	}
	return 0
}

// lineIndexFor builds the map inspect uses.
func lineIndexFor(source string) lineIndex { return newLineIndex(source) }
