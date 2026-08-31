package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
	"github.com/bhouse-nexthop/openpsirt/internal/signin"
)

// pendingCookie holds what a sign-in has to remember while the browser is away
// at the provider.
//
// Held by the browser rather than here. The alternative is a table of
// half-finished sign-ins, which has to be swept and which anybody can fill by
// starting sign-ins they never come back from.
const pendingCookie = "openpsirt_pending"

// pendingLife bounds how long a sign-in may take.
//
// Long enough to type a password and answer a second factor, short enough that
// an abandoned sign-in is not still usable later. Nothing is signed in during
// this window: what the cookie holds is the makings of a sign-in, not one.
const pendingLife = 10 * time.Minute

// registerSignIn mounts the sign-in paths on the router directly rather than
// through the API description.
//
// They are browser redirects rather than API operations: nothing calls them
// with a client, the response is a 302 with cookies, and describing them as
// operations would put two routes in the document that no generated client
// could ever usefully call.
func registerSignIn(router chi.Router, in Ingest) {
	if len(in.Providers) == 0 {
		return
	}
	router.Get("/v1/sign-in/{provider}", func(w http.ResponseWriter, r *http.Request) {
		begin(w, r, in)
	})
	router.Get("/v1/sign-in/{provider}/callback", func(w http.ResponseWriter, r *http.Request) {
		complete(w, r, in)
	})
}

// begin sends the browser to the provider.
func begin(w http.ResponseWriter, r *http.Request, in Ingest) {
	provider, ok := in.Providers[chi.URLParam(r, "provider")]
	if !ok {
		// The same answer a provider nobody configured gets, so that guessing
		// names does not enumerate which are in use.
		http.NotFound(w, r)
		return
	}

	callback, err := in.redirectURI(r, provider.Name())
	if err != nil {
		wentWrongHere(w, in, "a sign-in could not be started", err)
		return
	}
	where, pending, err := provider.Begin(r.Context(), callback)
	if err != nil {
		wentWrongHere(w, in, "a sign-in could not be started", err)
		return
	}

	encoded, err := json.Marshal(pending)
	if err != nil {
		wentWrongHere(w, in, "a sign-in could not be started", err)
		return
	}
	cookie := browserCookie(pendingCookie,
		base64.RawURLEncoding.EncodeToString(encoded), false, in.PlainHTTP, int(pendingLife.Seconds()))
	http.SetCookie(w, &cookie)
	// Where this goes is the provider's authorization endpoint, and an adapter
	// refuses at startup to be built around one that is not on the configured
	// provider's own host. That check is there rather than here because a
	// provider that would misdirect people should stop the process, not
	// misdirect the first person to sign in.
	http.Redirect(w, r, where, http.StatusFound) //nolint:gosec // G710: the target is a provider endpoint validated against its configured issuer at startup.
}

// complete turns what the provider sent back into a session.
func complete(w http.ResponseWriter, r *http.Request, in Ingest) {
	provider, ok := in.Providers[chi.URLParam(r, "provider")]
	if !ok {
		http.NotFound(w, r)
		return
	}

	pending, err := pendingFrom(r)
	if err != nil {
		refuseSignIn(w, in)
		return
	}
	// Compared before anything is exchanged. The state is what stops somebody
	// handing a signed-in person a callback of their own making and having the
	// session come back as theirs.
	if state := r.URL.Query().Get("state"); state == "" || state != pending.State {
		refuseSignIn(w, in)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		refuseSignIn(w, in)
		return
	}

	callback, err := in.redirectURI(r, provider.Name())
	if err != nil {
		wentWrongHere(w, in, "a sign-in could not be completed", err)
		return
	}
	identity, err := provider.Complete(r.Context(), code, pending, callback)
	if err != nil {
		// Logged rather than described. What went wrong between us and a
		// provider is an operator's problem, and telling whoever is at the
		// browser would describe our configuration to them.
		if in.Logger != nil {
			in.Logger.Warn("a sign-in could not be completed",
				"provider", provider.Name(), "error", err)
		}
		refuseSignIn(w, in)
		return
	}

	if in.DB == nil {
		refuseSignIn(w, in)
		return
	}
	rights := access.NewStore(in.DB.DB)

	// Authenticating is not being authorized, and which of the two questions
	// gets asked here depends on where this deployment says roles come from.
	//
	// Assigned: this looks somebody up and cannot bring them into being, so
	// somebody who signs in perfectly well and was never granted anything is
	// refused (ACC-21).
	//
	// Derived: the mapping an administrator made *is* the advance
	// authorization, so somebody arriving for the first time in a mapped group
	// is admitted and recorded then — and somebody in no mapped group is
	// refused exactly as a stranger is (ACC-27).
	person, err := admit(r, in, rights, provider.Name(), identity)
	if err != nil {
		if in.Logger != nil {
			in.Logger.Info("refused somebody who authenticated but reaches nothing",
				"provider", provider.Name(), "identity", identity.Username)
		}
		refuseSignIn(w, in)
		return
	}

	// The administrator's setting first, then whatever the deployment was
	// started with, then the built-in. The setting is offered on the
	// administration screen, so it has to be the one that decides — a value
	// somebody sets there and nothing reads is worse than not offering it.
	lifetime := access.DefaultSessionLifetime
	if in.SessionLifetime > 0 {
		lifetime = in.SessionLifetime
	}
	if in.DB != nil {
		chosen, err := setting.NewStore(in.DB.DB).Duration(r.Context(), setting.SessionLifetime, lifetime)
		if err != nil {
			wentWrongHere(w, in, "how long a sign-in lasts could not be read", err)
			return
		}
		lifetime = chosen
	}
	issued, err := rights.StartSession(r.Context(), person.ID, lifetime)
	if err != nil {
		wentWrongHere(w, in, "a session could not be started", err)
		return
	}

	session := SessionCookie(issued.Token, in.PlainHTTP)
	csrf := CSRFCookie(issued.CSRF, in.PlainHTTP)
	cleared := browserCookie(pendingCookie, "", false, in.PlainHTTP, -1)
	http.SetCookie(w, &session)
	http.SetCookie(w, &csrf)
	http.SetCookie(w, &cleared)
	http.Redirect(w, r, "/", http.StatusFound)
}

// admit decides whether somebody who authenticated may be here, in whichever
// way this deployment assigns roles.
func admit(r *http.Request, in Ingest, rights *access.Store, provider string, identity *signin.Identity) (*access.Account, error) {
	// Matched on the provider's own stable identifier, with the username only
	// redeeming an authorization nobody has pinned yet. A username moves —
	// people rename themselves, and a forge login left behind can be taken by
	// somebody else — so matching on it would eventually hand one person's
	// access to another.
	who := access.Arrival{
		Provider: provider, Subject: identity.Subject,
		Username: identity.Username, DisplayName: identity.DisplayName,
	}

	if in.Mode != nil && in.Mode(r.Context()) == access.GroupBound {
		if _, err := rights.AdmitByGroups(r.Context(), who, identity.Groups); err != nil {
			return nil, err
		}
		return rights.MatchProvider(r.Context(), who.Provider, who.Subject, who.Username)
	}

	person, err := rights.MatchProvider(r.Context(), who.Provider, who.Subject, who.Username)
	if err != nil {
		return nil, err
	}
	if _, err := rights.Resolve(r.Context(), person.Identity); err != nil {
		return nil, err
	}
	return person, nil
}

// pendingFrom reads back what the sign-in remembered.
func pendingFrom(r *http.Request) (signin.Pending, error) {
	cookie, err := r.Cookie(pendingCookie)
	if err != nil || cookie.Value == "" {
		return signin.Pending{}, errors.New("no sign-in is in progress")
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return signin.Pending{}, err
	}
	var pending signin.Pending
	if err := json.Unmarshal(raw, &pending); err != nil {
		return signin.Pending{}, err
	}
	if pending.State == "" || pending.Verifier == "" {
		return signin.Pending{}, errors.New("the sign-in in progress is incomplete")
	}
	return pending, nil
}

// redirectURI is where the provider sends the browser back to.
//
// Built from the configured base address where there is one. A provider
// compares this against what it was registered with, so it has to be the
// address people actually arrive on rather than whatever this process thinks
// it is called — behind a proxy those differ.
func (in Ingest) redirectURI(r *http.Request, provider string) (string, error) {
	base := strings.TrimSuffix(in.BaseURL, "/")
	if base == "" {
		// The Host header is whatever the caller sent. Building the address a
		// provider will send somebody back to out of it means a request
		// claiming another host produces an authorization URL pointing there,
		// and whether that is exploitable depends entirely on how strictly the
		// provider matches its registered addresses — which is not ours to
		// assume.
		//
		// So a deployment that configured a provider has to say where it is
		// served. It is one setting, it is already needed for the provider's
		// own registration to match, and failing here is visible where the
		// alternative is not.
		return "", errors.New("this deployment has not been told the address it is served on")
	}
	return base + "/v1/sign-in/" + provider + "/callback", nil
}

// refuseSignIn answers a sign-in that will not be completed.
//
// One answer for every reason: a provider that failed, a state that did not
// match, somebody unknown, and somebody known but granted nothing. Telling
// them apart says whether a name is real, which is free reconnaissance for
// somebody who has just proved nothing (ACC-21).
func refuseSignIn(w http.ResponseWriter, in Ingest) {
	cleared := browserCookie(pendingCookie, "", false, in.PlainHTTP, -1)
	http.SetCookie(w, &cleared)
	http.Error(w, "not authorized", http.StatusUnauthorized)
}

// wentWrongHere records a fault where an operator can read it and says nothing
// about it to whoever asked.
func wentWrongHere(w http.ResponseWriter, in Ingest, what string, err error) {
	if in.Logger != nil {
		in.Logger.Error(what, "error", err)
	}
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}
