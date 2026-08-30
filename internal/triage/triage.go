// Package triage holds what people decide about findings, and the rules for
// when a decision stops applying.
//
// The shape everything here rests on: a decision is a claim about a
// combination of code, not about a release. It is keyed structurally — which
// issue, in which component, under which consumer — and it stops applying when
// the code it was about changes. Those two are kept apart deliberately:
// identity is structural and expiry is version-based, and letting either reach
// into the other is how an unrelated bump at the top of a build invalidates a
// judgment made about a leaf.
package triage

import "strings"

// Outcome is what somebody decided about a finding.
//
// Four rather than two. A vocabulary with only "affects us" and "does not"
// has nowhere to put the most common real answer, which is "yes, but not now"
// — and the absence shows up as people recording it as one of the other two,
// after which no report can tell the difference.
type Outcome string

const (
	// Affected means it applies and goes to remediation.
	Affected Outcome = "affected"
	// NotApplicable means it does not affect this product here.
	NotApplicable Outcome = "not-applicable"
	// Deferred means it affects us and is not being worked on until a date.
	Deferred Outcome = "deferred"
	// WontFix means it affects us and will not be addressed.
	WontFix Outcome = "wont-fix"
)

// Outcomes are the four, in the order a person meets them.
func Outcomes() []Outcome { return []Outcome{Affected, NotApplicable, Deferred, WontFix} }

// Valid reports whether o is one we recognize.
func (o Outcome) Valid() bool {
	for _, known := range Outcomes() {
		if o == known {
			return true
		}
	}
	return false
}

// HidesRisk reports whether recording this takes something out of the working
// queue.
//
// The distinction the review queue is built on: hiding risk needs a second
// person, and putting it back does not. "Affected" is the only one of the four
// that leaves the issue visible as an issue.
func (o Outcome) HidesRisk() bool { return o != Affected }

// Exported says what an outcome is published as.
//
// A deferral exports as affected, never as not-affected. Deferring is an
// internal scheduling judgment; publishing it as not-affected would tell the
// outside world we assessed something as harmless when we had only put it off.
func (o Outcome) Exported() Outcome {
	if o == Deferred {
		return Affected
	}
	return o
}

// Justification is why something does not affect us.
//
// The vocabulary is the one the exchange format already defines rather than
// one of ours. It encodes exactly this reasoning, it is what a consumer of our
// published statements will expect, and using it makes publishing them close
// to free — whereas a private vocabulary would need a mapping that nobody
// maintains and that loses meaning at every step.
type Justification string

const (
	// ComponentNotPresent means the component is not in what ships, whatever
	// the inventory says.
	ComponentNotPresent Justification = "component_not_present"
	// CodeNotPresent means the component ships without the vulnerable code.
	CodeNotPresent Justification = "vulnerable_code_not_present"
	// CodeNotInExecutePath means the vulnerable code ships and never runs.
	CodeNotInExecutePath Justification = "vulnerable_code_not_in_execute_path"
	// CodeNotReachableByAdversary means it runs but nothing an attacker
	// controls reaches it.
	CodeNotReachableByAdversary Justification = "vulnerable_code_cannot_be_controlled_by_adversary"
	// MitigationsExist means something already in place stops it.
	MitigationsExist Justification = "inline_mitigations_already_exist"
)

// Justifications are the recognized categories.
func Justifications() []Justification {
	return []Justification{
		ComponentNotPresent, CodeNotPresent, CodeNotInExecutePath,
		CodeNotReachableByAdversary, MitigationsExist,
	}
}

// Valid reports whether j is one we recognize.
func (j Justification) Valid() bool {
	for _, known := range Justifications() {
		if j == known {
			return true
		}
	}
	return false
}

// AsJustification reads a stated category, tolerating the spellings that
// arrive from elsewhere.
//
// Producers and people write these with either separator and in either case.
// Refusing a claim over a hyphen would throw away the reasoning it carried.
func AsJustification(s string) (Justification, bool) {
	normalized := Justification(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "_"))
	return normalized, normalized.Valid()
}

// State is where a decision has got to.
//
// Append-only in spirit: a decision moves forward and what it was before stays
// readable, so the record reads as proposed, approved, withdrawn rather than
// as whatever it happens to be now.
type State string

const (
	// Proposed means somebody has claimed it and nobody has agreed yet.
	Proposed State = "proposed"
	// Approved means a second person agreed, against one specific revision of
	// the reasoning.
	Approved State = "approved"
	// Withdrawn means it no longer applies because somebody took it back.
	Withdrawn State = "withdrawn"
	// Lapsed means the code it was a claim about changed.
	Lapsed State = "lapsed"
)
