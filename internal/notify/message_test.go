package notify

import (
	"strings"
	"testing"
)

func TestAMessageAboutSomethingUndisclosedSaysOnlyThatThereIsSomething(t *testing.T) {
	// NTF-15. A message cannot re-check its reader: it sits on a mail server
	// this deployment does not run, in an inbox, in a phone's lock screen, and
	// in whatever it is forwarded to. So an alert about a flaw nobody has
	// announced is itself the announcement unless it says nothing.
	//
	// The body here is the one the notification area shows, which names
	// everything — that is what makes this worth pinning: the detail exists
	// and must not travel.
	n := Notification{
		Kind:    DisclosureDue,
		Private: true,
		Body: "SONIC-2026-0001 in sonic reached its disclosure date on 2026-09-04 " +
			"and nothing has been decided.",
		Link: "/products/sonic/streams/master/variants/broadcom/findings/SONIC-2026-0001/components/swss",
	}
	got := Compose(n, "https://psirt.example")

	for _, leaked := range []string{
		"SONIC-2026-0001", "sonic", "swss", "2026-09-04", "disclosure date",
	} {
		if strings.Contains(got.Text, leaked) {
			t.Errorf("the message carries %q, which is the announcement it exists to avoid:\n%s",
				leaked, got.Text)
		}
		if strings.Contains(got.Subject, leaked) {
			t.Errorf("the subject carries %q, and a preview shows a subject without "+
				"anybody opening anything: %q", leaked, got.Subject)
		}
	}
	// The way in is the message, and it is the front door rather than the
	// finding: a path naming the identifier and the component announces both
	// to every server the message crosses, whatever the body withheld.
	if !strings.Contains(got.Text, "https://psirt.example/\n") {
		t.Errorf("the message carries no way in:\n%s", got.Text)
	}
	if strings.Contains(got.Text, "/findings/") {
		t.Errorf("the address names the finding, which the body was careful not to:\n%s", got.Text)
	}
}

func TestAMessageAboutSomethingPublicCarriesWhatItIsAbout(t *testing.T) {
	// The other half of NTF-15, and as important: a public vulnerability in a
	// shipped component is public. A channel that says nothing about anything
	// is one people stop reading, and then the embargo messages go unread with
	// the rest.
	n := Notification{
		Kind: Assigned,
		Body: "CVE-2025-60876 in busybox-binsh, in openpsirt main container",
		Link: "/products/openpsirt/streams/main/variants/container/findings/CVE-2025-60876",
	}
	got := Compose(n, "https://psirt.example")

	for _, wanted := range []string{"CVE-2025-60876", "busybox-binsh", "openpsirt"} {
		if !strings.Contains(got.Text, wanted) {
			t.Errorf("the message does not say %q, which is what makes it worth opening:\n%s",
				wanted, got.Text)
		}
	}
	if !strings.Contains(got.Text, "https://psirt.example/products/openpsirt/") {
		t.Errorf("the message carries no way to reach it:\n%s", got.Text)
	}
}

func TestAMessageWithNowhereToPointStillSaysWhatItCameToSay(t *testing.T) {
	// A deployment that has not been told the address people arrive on cannot
	// make a link, and half an address is worse than none: it looks like it
	// works and lands nowhere.
	n := Notification{Kind: Assigned, Body: "CVE-2025-1 in libfoo", Link: "/findings/1"}
	got := Compose(n, "")
	if strings.Contains(got.Text, "http") {
		t.Errorf("a deployment with no address invented one:\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "CVE-2025-1") {
		t.Errorf("the message lost what it was about:\n%s", got.Text)
	}
}

func TestASubjectSaysWhatKindOfThingAndNeverWhichOne(t *testing.T) {
	// A subject is the part shown without anybody opening anything, so it is
	// the same words whether or not the thing behind it is disclosed.
	public := Compose(Notification{Kind: Assigned, Body: "CVE-2025-1 in libfoo"}, "")
	private := Compose(Notification{Kind: Assigned, Body: "SONIC-2026-1 in swss", Private: true}, "")
	if public.Subject != private.Subject {
		t.Errorf("the subject differs by what it is about: %q against %q",
			public.Subject, private.Subject)
	}
	if public.Subject == "" {
		t.Error("a message with no subject is one a reader cannot triage in a list")
	}
}
