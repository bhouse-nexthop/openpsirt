// Package access answers who is asking and what they may reach.
//
// Two things are kept apart deliberately. Authenticating establishes who
// somebody is; it says nothing about whether they should be here. So no path
// through this package creates an account, and somebody who authenticates
// perfectly well but was never granted anything is turned away with the same
// answer as somebody unknown — telling an outsider which of the two applies is
// free reconnaissance.
package access

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Role is what somebody may do with a product.
type Role string

const (
	// Reporting and Approver are capabilities rather than grants of
	// visibility: what they reach is still bounded by what the person may
	// read. Otherwise handing somebody the ability to approve would quietly
	// hand them everything there is to approve.
	Reporting Role = "reporting"
	Approver  Role = "approver"

	// The reading and triage roles, per visibility.
	PublicRead    Role = "public-read"
	PrivateRead   Role = "private-read"
	PublicTriage  Role = "public-triage"
	PrivateTriage Role = "private-triage"
)

// Roles are the baseline set. It is a floor, not a ceiling.
func Roles() []Role {
	return []Role{Reporting, Approver, PublicRead, PrivateRead, PublicTriage, PrivateTriage}
}

// Valid reports whether r is a role we recognize.
func (r Role) Valid() bool {
	for _, known := range Roles() {
		if r == known {
			return true
		}
	}
	return false
}

// Visibility says whether something has been disclosed.
//
// It is about disclosure, not about who may read: every request is
// authenticated either way, so a mistake in these rules exposes something to a
// colleague rather than to the internet.
type Visibility string

const (
	// Public means disclosed.
	Public Visibility = "public"
	// Private means not yet disclosed.
	Private Visibility = "private"
)

// AsVisibility reads a stored value, treating anything unrecognized as not
// disclosed. Unset has to read as private, or a column added later defaults
// every existing row to visible.
func AsVisibility(s string) Visibility {
	if Visibility(s) == Public {
		return Public
	}
	return Private
}

// Kind distinguishes the sorts of thing that can be asking.
type Kind string

const (
	// Person is somebody who signed in.
	Person Kind = "person"
	// Pipeline is a build authenticating with a key. It may send scans and do
	// nothing else — no reading, no triage, no reporting — which is what keeps
	// the visibility rules out of a build server's reach entirely.
	Pipeline Kind = "pipeline"
)

// ErrNoSubject is returned when a query is attempted with nobody attached.
//
// It is an error rather than a denial because it is a fault in this program:
// somewhere a query was written that does not say who is asking, and answering
// it would be answering for everybody.
var ErrNoSubject = errors.New("no subject: a query was attempted without saying who is asking")

// ErrDenied is what somebody unauthorized is told.
//
// Deliberately the same whether they are unknown, known but granted nothing, or
// granted something that does not cover this. Telling an outsider which of
// those applies is free reconnaissance.
var ErrDenied = errors.New("not authorized")

// Subject is who is asking.
type Subject struct {
	Kind Kind
	// ID is the person or the key, depending on the kind.
	ID int64
	// Identity is what to call them in a record of what was done.
	Identity string
	// Admin is global and belongs to a person. It is the one role not granted
	// against a product.
	Admin bool
	// grants is what this person may do, per product.
	grants map[int64][]Role
	// scope is what a pipeline's key allows. Absent for a person.
	scope *Scope
}

// Scope is the set of constraints on a key.
//
// The product is always required. The release and the variant are independent,
// and either, both or neither may be pinned — so a key covers a product, a
// product and a variant, a product and a release, or all three.
type Scope struct {
	ProductID int64
	StreamID  *int64
	VariantID *int64
}

// NewPerson returns the subject for somebody who signed in.
func NewPerson(id int64, identity string, admin bool, grants map[int64][]Role) Subject {
	return Subject{Kind: Person, ID: id, Identity: identity, Admin: admin, grants: grants}
}

// NewPipeline returns the subject for a build authenticating with a key.
func NewPipeline(id int64, name string, scope Scope) Subject {
	return Subject{Kind: Pipeline, ID: id, Identity: name, scope: &scope}
}

// Holds reports whether this subject holds a role on a product.
//
// An admin holds every role everywhere. That is what being an admin is, and
// checking it in one place rather than at every call site is what stops one
// forgotten check from being an escalation.
func (s Subject) Holds(role Role, productID int64) bool {
	if s.Kind != Person {
		return false
	}
	if s.Admin {
		return true
	}
	for _, held := range s.grants[productID] {
		if held == role {
			return true
		}
	}
	return false
}

// Reads reports whether this subject may read something of this visibility in
// this product.
//
// Triage implies reading at the same visibility: somebody who may decide about
// a finding can necessarily see it, and a deployment that had to grant both
// would eventually grant one.
func (s Subject) Reads(visibility Visibility, productID int64) bool {
	switch visibility {
	case Public:
		return s.Holds(PublicRead, productID) || s.Holds(PublicTriage, productID) ||
			s.Holds(PrivateRead, productID) || s.Holds(PrivateTriage, productID)
	default:
		return s.Holds(PrivateRead, productID) || s.Holds(PrivateTriage, productID)
	}
}

// Sees reports whether this subject may know a product exists.
//
// A product somebody holds nothing on is invisible rather than merely
// unreadable — not listed and not counted — because the list of products is
// itself a statement about what an organization ships.
func (s Subject) Sees(productID int64) bool {
	if s.Kind == Pipeline && s.scope != nil {
		// A pipeline knows the product it may send to exists, because it may
		// send to it. It knows nothing about any other.
		return s.scope.ProductID == productID
	}
	if s.Kind != Person {
		return false
	}
	if s.Admin {
		return true
	}
	// A role that grants reading, not merely any role. A capability is
	// bounded by what its holder may read, so holding one alone is not a way
	// in — otherwise granting somebody the ability to approve would show them
	// every release and variant there is to approve.
	return s.Reads(Public, productID) || s.Reads(Private, productID)
}

// Products returns the products this subject may know about. An admin sees
// everything, which is reported as nothing listed rather than as a list.
func (s Subject) Products() (ids []int64, all bool) {
	if s.Kind != Person {
		return nil, false
	}
	if s.Admin {
		return nil, true
	}
	for id := range s.grants {
		if s.Sees(id) {
			ids = append(ids, id)
		}
	}
	return ids, false
}

// MaySend reports whether a pipeline's key authorizes an upload against this
// exact target.
//
// Every constraint the key carries must match. A mismatch is refused rather
// than redirected: a key that covers one release must not quietly accept a
// scan of another, and an upload states its full target explicitly so there is
// never anything to infer.
func (s Subject) MaySend(productID, streamID, variantID int64) bool {
	if s.Kind != Pipeline || s.scope == nil {
		return false
	}
	if s.scope.ProductID != productID {
		return false
	}
	if s.scope.StreamID != nil && *s.scope.StreamID != streamID {
		return false
	}
	if s.scope.VariantID != nil && *s.scope.VariantID != variantID {
		return false
	}
	return true
}

// sessionKey carries the browser session a request arrived on, where it
// arrived on one. It is separate from the subject because most requests have
// no session — a pipeline's key and a proxy's header both resolve to a subject
// without one — and because what it is for is narrow: deciding whether a
// state-changing request was made by our own page or by somebody else's.
type sessionKey struct{}

// WithSession attaches the session a request arrived on.
func WithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, session)
}

// SessionFrom returns the session a request arrived on, if it arrived on one.
func SessionFrom(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionKey{}).(*Session)
	return session
}

// contextKey is unexported so nothing outside this package can put a subject
// into a context by accident, or take one out without going through here.
type contextKey struct{}

// With attaches a subject to a context.
func With(ctx context.Context, s Subject) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// From reads the subject a request resolved to.
//
// It fails rather than defaulting. A query that reaches the database without
// saying who is asking is a bug in this program, and the safe-looking
// alternative — treating absence as "nobody, so show nothing" — hides it until
// somebody writes the query that treats absence as "everybody".
func From(ctx context.Context) (Subject, error) {
	s, ok := ctx.Value(contextKey{}).(Subject)
	if !ok {
		return Subject{}, ErrNoSubject
	}
	return s, nil
}

// Identities normalizes a configured list of people.
func Identities(raw string) []string {
	var out []string
	for _, name := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Denied wraps a refusal with what was being attempted, for the log rather
// than for the person.
func Denied(what string) error { return fmt.Errorf("%w: %s", ErrDenied, what) }
