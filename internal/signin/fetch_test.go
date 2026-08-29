package signin

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAProviderClientTalksToItsOwnHostAndNowhereElse(t *testing.T) {
	// The addresses this client fetches come from configuration and from a
	// provider's own discovery document, which is to say from outside. An
	// unrestricted client pointed at a discovery document is a request forgery
	// primitive.
	client := guardedClient("issuer.example")

	for _, url := range []string{
		"https://elsewhere.example/keys",
		"https://issuer.example.attacker.test/keys",
		"https://attacker.test/?x=issuer.example",
	} {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Transport.RoundTrip(req); err == nil {
			t.Errorf("%s was allowed", url)
		} else if !strings.Contains(err.Error(), "not a configured provider host") {
			t.Errorf("%s was refused for the wrong reason: %v", url, err)
		}
	}
}

func TestAProviderIsReachedOverHTTPSOnly(t *testing.T) {
	// A provider reached over plain http can be answered by anybody on the
	// path, and what comes back decides who somebody is.
	client := guardedClient("issuer.example")
	req, err := http.NewRequest(http.MethodGet, "http://issuer.example/.well-known/openid-configuration", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The reason matters, not just the failure: without the scheme check this
	// request goes on to fail at DNS, which would let the test pass while the
	// guard it names does nothing.
	_, err = client.Transport.RoundTrip(req)
	if err == nil {
		t.Fatal("a provider was reached over plain http")
	}
	if !strings.Contains(err.Error(), "reached over https") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

func TestAProviderIsNotFollowedToSomewhereElse(t *testing.T) {
	// A redirect is the provider telling us to fetch somewhere else, and
	// somewhere else is the thing being guarded against.
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "should never be reached", http.StatusTeapot)
	}))
	defer elsewhere.Close()

	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer moved.Close()

	// Pointed at the test server directly, so what is being measured is the
	// redirect policy rather than the host and scheme guards.
	client := guardedClient()
	client.Transport = http.DefaultTransport
	if _, err := client.Get(moved.URL); err == nil {
		t.Error("a redirect was followed")
	} else if !strings.Contains(err.Error(), "refused a redirect") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

func TestTheAddressGuardSeesAnAddressAndNotAName(t *testing.T) {
	// The test whose absence hid a total failure. Everything else here calls
	// reachable() with address literals, so nothing noticed that a request
	// through the client never got that far: a dialer is handed the
	// *unresolved* name from the URL, and a check written against an address
	// refuses every name — including every real provider's.
	//
	// A name is what distinguishes the two. The URL below names a host rather
	// than an address, so a guard that runs before resolution can only say
	// "not an address", while one that runs after says what it actually
	// found. Asserting on which of those comes back is the whole point.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a request reached a server inside this network")
	}))
	defer server.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	client := guardedClient("localhost")

	_, err = client.Get("https://localhost:" + port)
	if err == nil {
		t.Fatal("a request to this machine was allowed")
	}
	if strings.Contains(err.Error(), "not an address") {
		t.Fatalf("the guard ran before the name was resolved, so it refuses every provider: %v", err)
	}
	if !strings.Contains(err.Error(), "not reached inside this network") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

func TestAnAllowedProviderSurvivesEveryLayerOfTheClient(t *testing.T) {
	// A guard that refuses everything would pass every refusal test in this
	// file. This is the other direction: a request the client is meant to
	// allow has to actually come back.
	//
	// The server is on this machine, which the address guard refuses by
	// design, so that one check is stood down and nothing else is — scheme,
	// host allowlist and redirect policy all stay as a deployment gets them.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issuer":"https://issuer.example"}`))
	}))
	defer server.Close()

	host, _, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	client := guardedClient(host)
	transport, ok := client.Transport.(*guard).inner.(*http.Transport)
	if !ok {
		t.Fatal("the guarded client is not built the way this test assumes")
	}
	transport.DialContext = (&net.Dialer{}).DialContext
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("the guarded client could not reach a provider it was allowed to reach: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("answered %s", resp.Status)
	}
}

func TestAProviderIsNeverReachedInsideThisNetwork(t *testing.T) {
	// A name that resolves inward is either a mistake or something somebody
	// arranged, and neither is a thing to connect to and hand a request.
	for _, address := range []string{
		"127.0.0.1:443",
		"[::1]:443",
		"10.0.0.5:443",
		"192.168.1.1:443",
		"172.16.0.1:443",
		"169.254.169.254:443", // the address cloud metadata services answer on
		"0.0.0.0:443",
	} {
		if err := reachable(address); err == nil {
			t.Errorf("%s was allowed", address)
		}
	}
}

func TestAProviderOnTheInternetIsReached(t *testing.T) {
	// The guard has to let the ordinary case through, or every sign-in fails
	// and the refusals above prove nothing.
	for _, address := range []string{"140.82.121.4:443", "[2606:50c0:8000::153]:443"} {
		if err := reachable(address); err != nil {
			t.Errorf("%s was refused: %v", address, err)
		}
	}
}

func TestAProofKeyIsSentAsADigestAndKeptAsTheSecret(t *testing.T) {
	// The provider only ever sees the digest, so an authorization code
	// intercepted on its way back cannot be exchanged by whoever took it.
	first, err := newPending()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newPending()
	if err != nil {
		t.Fatal(err)
	}

	if first.challenge() == first.Verifier {
		t.Error("the proof key was sent as the secret it is meant to hash")
	}
	if first.State == second.State || first.Nonce == second.Nonce || first.Verifier == second.Verifier {
		t.Error("two sign-ins were given the same values")
	}
	if first.State == first.Nonce || first.State == first.Verifier {
		t.Error("one value is doing the work of two")
	}
	for what, value := range map[string]string{
		"state": first.State, "nonce": first.Nonce, "verifier": first.Verifier,
	} {
		if len(value) < 40 {
			t.Errorf("the %s is %d characters, which is not enough to be unguessable", what, len(value))
		}
	}
}

func TestGroupMembershipThatCannotBeReadYieldsNoRolesRatherThanEveryRole(t *testing.T) {
	// The failure that would otherwise be silent and total.
	for _, claim := range []any{nil, 42, map[string]any{"a": 1}, []any{1, 2}, "", "   "} {
		if got := groups(claim); len(got) != 0 {
			t.Errorf("%#v read as %v, want nothing", claim, got)
		}
	}
	for _, claim := range []any{
		[]any{"platform", "security"},
		[]string{"platform", "security"},
	} {
		if got := groups(claim); len(got) != 2 || got[0] != "platform" || got[1] != "security" {
			t.Errorf("%#v read as %v", claim, got)
		}
	}
	if got := groups("platform"); len(got) != 1 || got[0] != "platform" {
		t.Errorf("a single group as a bare string read as %v", got)
	}
}

func TestAProviderThatNamesNobodyIsAMisconfigurationRatherThanAnAnonymousUser(t *testing.T) {
	// An invented username would match nothing an administrator granted, and
	// the sign-in would be refused for a reason nobody could work out.
	if _, err := usernameFrom("", "  ", ""); err == nil {
		t.Error("a provider that supplied no username produced one anyway")
	}
	got, err := usernameFrom("", " someone ", "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if got != "someone" {
		t.Errorf("picked %q", got)
	}
}
