// Command unclaimed reports decisions that no design document names.
//
// The chain this repository is organized around runs one way: code satisfies a
// design document, a design document names the decisions it implements, and a
// decision says why. That chain is what makes an audit possible, and until now
// nothing checked that it was whole. It was not: 46 decisions in force were
// named by no design document at all, including five the code itself cites —
// behaviour that runs, is reasoned about in comments, and appears in no
// document describing the system.
//
// A decision that is not built yet is not exempt. The design document for its
// area says so, in words, the way several already do — "release over release
// is not built", "publication is entirely Phase 2". That is the difference
// between a gap somebody rediscovers by clicking and a plan somebody can read.
//
// Ranges count. A document that says "satisfies TRI-01 to TRI-03" has named
// all three, which is how the existing documents are written.
//
// Deliberately crude, like the unreachable gate beside it: it matches
// identifiers rather than understanding them, so naming a decision anywhere in
// a design document counts as naming it. That errs toward saying nothing,
// which is the right direction for a check that fails a build.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Everything is read through the working directory as a file system rather
// than by path. A build-time tool that reads wherever it is pointed is a shape
// worth not having, and stating it this way makes it structural instead of a
// convention somebody has to keep — the analysis gate is right to complain
// about the other spelling.
var here = os.DirFS(".")

// Section 3 of DECISIONS.md holds the decisions in force. Sections 4 and after
// hold what is still open, what was rejected and what changed, none of which a
// design document is expected to implement.
const (
	inForceFrom = "## 3. Decisions"
	inForceTo   = "## 4. Still open"
)

// A row in the decisions table: "| TRI-01 | ... | ... |".
var row = regexp.MustCompile(`(?m)^\| ([A-Z]{3}-\d+) \| (.*)$`)

// An identifier anywhere in prose, and the range form the documents use.
var (
	one   = regexp.MustCompile(`\b[A-Z]{3}-\d+\b`)
	spans = regexp.MustCompile(`\b([A-Z]{3})-(\d+)\s+to\s+([A-Z]{3})-(\d+)`)
)

func main() {
	decisions, err := fs.ReadFile(here, "DECISIONS.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "unclaimed:", err)
		os.Exit(2)
	}
	text := string(decisions)
	from, to := strings.Index(text, inForceFrom), strings.Index(text, inForceTo)
	if from < 0 || to < 0 || to < from {
		fmt.Fprintln(os.Stderr, "unclaimed: DECISIONS.md has no section of decisions in force")
		os.Exit(2)
	}

	// Every decision in force, in the order the document lists them, minus the
	// ones it records as withdrawn — a decision that was taken back is history
	// rather than an obligation, and asking a design document to describe it
	// would be asking for a description of something that is not there.
	var want []string
	width := map[string]int{}
	for _, match := range row.FindAllStringSubmatch(text[from:to], -1) {
		id, body := match[1], match[2]
		if strings.HasPrefix(strings.TrimSpace(strings.TrimLeft(body, "*")), "Withdrawn") {
			continue
		}
		want = append(want, id)
		if n := len(id) - 4; n > width[id[:3]] {
			width[id[:3]] = n
		}
	}

	named, err := namedByDesigns(width)
	if err != nil {
		fmt.Fprintln(os.Stderr, "unclaimed:", err)
		os.Exit(2)
	}

	var missing []string
	for _, id := range want {
		if !named[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		fmt.Printf("every one of the %d decisions in force is named by a design document\n", len(want))
		return
	}

	byArea := map[string][]string{}
	for _, id := range missing {
		byArea[id[:3]] = append(byArea[id[:3]], id)
	}
	areas := make([]string, 0, len(byArea))
	for area := range byArea {
		areas = append(areas, area)
	}
	sort.Strings(areas)

	fmt.Fprintf(os.Stderr, "%d of %d decisions in force are named by no design document:\n",
		len(missing), len(want))
	for _, area := range areas {
		fmt.Fprintf(os.Stderr, "  %s  %s\n", area, strings.Join(byArea[area], " "))
	}
	fmt.Fprintln(os.Stderr, "\nA decision is named where the document that describes its area says what")
	fmt.Fprintln(os.Stderr, "it does — or, where it is not built, says that. Neither is optional: the")
	fmt.Fprintln(os.Stderr, "chain from code to design to reasoning is what makes this auditable.")
	os.Exit(1)
}

// namedByDesigns collects every decision identifier the design documents name.
//
// Design documents only. A decision named in a commit message, a code comment
// or a plan is not described anywhere permanent — the plan is deleted when its
// work lands, and a comment describes one function rather than the system.
func namedByDesigns(width map[string]int) (map[string]bool, error) {
	designs, err := fs.Glob(here, "DESIGN-*.md")
	if err != nil {
		return nil, err
	}
	named := map[string]bool{}
	for _, path := range designs {
		body, err := fs.ReadFile(here, path)
		if err != nil {
			return nil, err
		}
		text := string(body)
		for _, id := range one.FindAllString(text, -1) {
			named[id] = true
		}
		// "TRI-01 to TRI-03" names everything between, which is how the
		// documents state a run of them.
		for _, span := range spans.FindAllStringSubmatch(text, -1) {
			if span[1] != span[3] {
				continue
			}
			first, _ := strconv.Atoi(span[2])
			last, _ := strconv.Atoi(span[4])
			for n := first; n <= last; n++ {
				named[fmt.Sprintf("%s-%0*d", span[1], width[span[1]], n)] = true
			}
		}
	}
	return named, nil
}
