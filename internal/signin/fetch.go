// Package signin turns a provider's answer into somebody we already know.
//
// Nothing here creates an account. A provider establishes who somebody is;
// whether they should be here was settled in advance by an administrator, so
// the end of every path in this package is a lookup that can fail.
package signin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// fetchTimeout bounds a call to a provider.
//
// Sign-in is interactive: somebody is watching a blank page while this runs.
// A provider that has stopped answering should fail the sign-in rather than
// hold a request open until something further up gives up on it.
const fetchTimeout = 10 * time.Second

// guardedClient returns an HTTP client that will only talk to the hosts named,
// will not follow a redirect, and will not connect to an address inside this
// network.
//
// All three matter because the addresses this client fetches come from
// configuration and from a provider's own discovery document, which is to say
// from outside. An unrestricted client pointed at a discovery document is a
// request forgery primitive: it would fetch whatever the document names, from
// inside the network, with whatever the network trusts this process to reach
// (SEC-07).
//
// Refusing redirects rather than following them to a checked host is
// deliberate. A redirect is the provider telling us to fetch somewhere else,
// and "somewhere else" is exactly what is being guarded against — a provider
// that genuinely moved its endpoints should be reconfigured, which is visible,
// rather than followed, which is not.
func guardedClient(hosts ...string) *http.Client {
	allowed := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		allowed[strings.ToLower(host)] = true
	}

	dialer := &net.Dialer{Timeout: fetchTimeout}
	return &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return fmt.Errorf("refused a redirect to %s: a provider's endpoints are configured, not followed", req.URL.Host)
		},
		Transport: &guard{
			allowed: allowed,
			inner: &http.Transport{
				DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					if err := reachable(address); err != nil {
						return nil, err
					}
					return dialer.DialContext(ctx, network, address)
				},
			},
		},
	}
}

// guard refuses a request to anywhere but the hosts a provider was configured
// with.
//
// Checked at the round trip rather than only at the dial, because the dial
// sees an address and this sees a name: a request for an unexpected host that
// happens to resolve to a permitted address would pass the first check and
// fail this one.
type guard struct {
	allowed map[string]bool
	inner   http.RoundTripper
}

func (g *guard) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("refused a request to %s: a provider is reached over https", req.URL)
	}
	if !g.allowed[strings.ToLower(req.URL.Hostname())] {
		return nil, fmt.Errorf("refused a request to %s: not a configured provider host", req.URL.Hostname())
	}
	return g.inner.RoundTrip(req)
}

// reachable refuses an address inside this network.
//
// A provider lives on the internet. An address that resolves to somewhere
// private is either a mistake in configuration or a name somebody arranged to
// point inward, and neither is something to connect to and hand a request.
func reachable(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// The dialer resolves before it gets here, so anything that is not an
		// address at this point is something unexpected rather than a name.
		return fmt.Errorf("refused a connection to %q: not an address", host)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("refused a connection to %s: a provider is not reached inside this network", ip)
	}
	return nil
}
