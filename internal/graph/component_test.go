package graph_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

func TestWhatAPackageWasBuiltFromIsReadFromItsIdentifier(t *testing.T) {
	for _, c := range []struct {
		purl    string
		name    string
		version string
	}{
		{"pkg:deb/debian/acl@2.3.2-2%2Bb1?arch=amd64&upstream=acl%402.3.2-2", "acl", "2.3.2-2"},
		// A binary cut from a differently named source package. The bare name
		// is the whole of what is knowable, and it is the half that matching a
		// build's own claims needs.
		{"pkg:deb/debian/apt-transport-https@3.0.3?arch=amd64&upstream=apt", "apt", ""},
		{"pkg:deb/debian/bsdextrautils@2.41.5?upstream=util-linux", "util-linux", ""},
		// An epoch contains a colon, and a version may contain more besides —
		// so the name is cut at the last separator, not the first.
		{"pkg:deb/debian/auditd@1:4.0.2-2%2Bb2?upstream=audit%401:4.0.2-2", "audit", "1:4.0.2-2"},
		// Nothing to say.
		{"pkg:deb/debian/acl@2.3.2-2", "", ""},
		{"pkg:deb/debian/acl@2.3.2-2?arch=amd64&distro=debian-13", "", ""},
		{"", "", ""},
		// A qualifier that only looks like it.
		{"pkg:deb/debian/acl@2.3.2-2?upstreamish=no", "", ""},
	} {
		name, version := graph.UpstreamFromPurl(c.purl)
		if name != c.name || version != c.version {
			t.Errorf("%s\n  read as (%q, %q), want (%q, %q)", c.purl, name, version, c.name, c.version)
		}
	}
}

func TestOneDescriptionFillsWhatAnotherLeftOut(t *testing.T) {
	// Two descriptions of one package, which is what a merged inventory
	// produces. What the first said stands — two producers disagreeing is not
	// something this can settle — and what it did not say is taken.
	kept := graph.Described{Name: "acl", Version: "2.3.2-2+b1"}
	kept.FillFrom(graph.Described{
		CPE: "cpe:2.3:a:acl:acl:2.3.2:*:*:*:*:*:*:*", UpstreamName: "acl", UpstreamVersion: "2.3.2-2",
	})
	if kept.CPE == "" || kept.UpstreamName != "acl" || kept.UpstreamVersion != "2.3.2-2" {
		t.Errorf("gaps were not filled: %+v", kept)
	}

	stated := graph.Described{
		Name: "acl", Version: "2.3.2-2+b1",
		CPE: "cpe:first", UpstreamName: "first", UpstreamVersion: "1",
	}
	stated.FillFrom(graph.Described{CPE: "cpe:second", UpstreamName: "second", UpstreamVersion: "2"})
	if stated.CPE != "cpe:first" || stated.UpstreamName != "first" || stated.UpstreamVersion != "1" {
		t.Errorf("a later description overwrote an earlier one: %+v", stated)
	}
}
