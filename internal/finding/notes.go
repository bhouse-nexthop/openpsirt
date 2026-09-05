package finding

import (
	"fmt"
	"sort"
	"strings"
)

// Notes renders a comparison as prose somebody pastes into a release note
// (RPT-06).
//
// **Markdown, and generated here rather than in a browser**, so that what an
// API caller gets and what the screen shows are the same words. A second
// implementation of "how a release note reads" is one that drifts, and the
// half that drifts is always the one nobody is looking at.
//
// The shape is three sections in the order somebody reading a release note
// cares about them: what was fixed, what appeared, and what is still there.
// Each line names the issue, what carries it, and — for a fix — how it was
// fixed, because "upgraded to 3.9.0" and "a carried patch" are different
// sentences to whoever reads this.
//
// **What is not a fix is not listed as one.** A bump that carried the issue
// with it closes a row and is `superseded`, and putting that under "Fixed"
// would be telling a customer something untrue in a document they keep.
func Notes(title string, c *Comparison) string {
	if c == nil {
		return ""
	}
	var out strings.Builder
	if title = strings.TrimSpace(title); title != "" {
		fmt.Fprintf(&out, "## Security fixes in %s\n", title)
	} else {
		out.WriteString("## Security fixes\n")
	}

	fixed, carried := split(c.Fixed)
	section(&out, "Fixed", fixed, func(row Changed) string {
		if said := fixedBecause(row.Because); said != "" {
			return " — " + said
		}
		return ""
	})
	// The bumps that fell short are listed apart from the fixes, because they
	// are the opposite answer to "was this fixed" and reading them together is
	// how one release note says both.
	section(&out, "Moved but not fixed", carried, func(Changed) string {
		return " — the version moved and the issue came with it"
	})
	section(&out, "Newly present", c.Newly, func(Changed) string { return "" })
	section(&out, "Still present", c.Still, func(row Changed) string {
		if row.ArrivedFrom != "" {
			return " — bumped from " + row.ArrivedFrom
		}
		return ""
	})

	if out.Len() == 0 {
		return ""
	}
	return out.String()
}

// split separates what was actually fixed from what merely moved.
func split(rows []Changed) (fixed, carried []Changed) {
	for _, row := range rows {
		if row.Because == Superseded || row.Because == Unexplained {
			carried = append(carried, row)
			continue
		}
		fixed = append(fixed, row)
	}
	return fixed, carried
}

// section writes one heading and its lines, or nothing where there are none.
//
// An empty heading in a release note is a question in the reader's mind about
// whether something is missing.
func section(out *strings.Builder, heading string, rows []Changed, note func(Changed) string) {
	if len(rows) == 0 {
		return
	}
	// Worst first, then by name, so two runs over the same pair of builds
	// produce the same document — a release note that reorders between reads
	// is one nobody can diff.
	ordered := append([]Changed(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := severityRank(ordered[i].Severity), severityRank(ordered[j].Severity)
		if a != b {
			return a > b
		}
		if ordered[i].Vulnerability != ordered[j].Vulnerability {
			return ordered[i].Vulnerability < ordered[j].Vulnerability
		}
		return ordered[i].Component < ordered[j].Component
	})

	fmt.Fprintf(out, "\n### %s\n\n", heading)
	for _, row := range ordered {
		fmt.Fprintf(out, "- %s in %s", row.Vulnerability, row.Component)
		if row.Severity != "" {
			fmt.Fprintf(out, " (%s)", row.Severity)
		}
		out.WriteString(note(row))
		out.WriteString("\n")
	}
}

// fixedBecause says how something was fixed, in the words a reader needs.
func fixedBecause(because Closure) string {
	switch because {
	case Upgraded:
		return "the component was upgraded"
	case Revised:
		return "a carried patch"
	case Removed:
		return "the component is no longer shipped"
	case Fixed:
		return "fixed here"
	}
	return ""
}

// severityRank orders the words, worst first.
func severityRank(word string) int {
	for i, known := range severityOrder {
		if strings.EqualFold(word, known) {
			return i
		}
	}
	return -1
}
