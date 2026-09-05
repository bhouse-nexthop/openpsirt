package markdown_test

import (
	"strings"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
)

const token = "0f9a1b2c3d4e5f60718293a4b5c6d7e8"

func TestAnImageMayComeFromAnAttachmentAndNowhereElse(t *testing.T) {
	// DESIGN-text.md's rule is unchanged and this is the whole of the change
	// to it: an image loaded from a third party reports who read a finding and
	// when, which on an undisclosed one is a disclosure channel. A file held
	// here is fetched through a path that asks who is looking.
	for _, c := range []struct {
		what    string
		source  string
		refused bool
	}{
		{"a file held here", "![a screenshot](attachment:" + token + ")", false},
		{"somebody else's server", "![a screenshot](https://example.org/s.png)", true},
		{"a data address", "![a screenshot](data:image/png;base64,AAAA)", true},
		{"a relative address", "![a screenshot](/static/s.png)", true},
		{"a link to a file held here", "[the log](attachment:" + token + ")", false},
	} {
		t.Run(c.what, func(t *testing.T) {
			err := markdown.Check(c.source)
			if c.refused && err == nil {
				t.Errorf("an image from %s was accepted", c.what)
			}
			if !c.refused && err != nil {
				t.Errorf("an image from %s was refused: %v", c.what, err)
			}
		})
	}
}

func TestARefusedImageSaysToAttachTheFile(t *testing.T) {
	// The refusal has to name the thing to do instead. Somebody told only that
	// their image is not allowed writes no image and no explanation either.
	err := markdown.Check("![shot](https://example.org/s.png)")
	if err == nil {
		t.Fatal("a remote image was accepted")
	}
	if !strings.Contains(err.Error(), "Attach the file") {
		t.Errorf("the refusal does not say what to do instead: %v", err)
	}
}

func TestReferencesFindsWhatTheTextActuallyPointsAt(t *testing.T) {
	other := "1122334455667788990011223344556677"[:32]
	source := "Evidence: ![shot](attachment:" + token + ")\n\n" +
		"And again ![same](attachment:" + token + ") and [a log](attachment:" + other + ")\n"
	got := markdown.References(source)
	if len(got) != 2 || got[0] != token || got[1] != other {
		t.Errorf("references are %v, want each one once in the order they appear", got)
	}
}

func TestTextShowingAReferenceDoesNotMakeOne(t *testing.T) {
	// Somebody explaining how to write one of these has not attached a file,
	// and counting it would keep an upload alive that nothing points at.
	for _, source := range []string{
		"Write it as `![shot](attachment:" + token + ")`\n",
		"```\n![shot](attachment:" + token + ")\n```\n",
	} {
		if got := markdown.References(source); len(got) != 0 {
			t.Errorf("a reference being shown was counted as one made: %v", got)
		}
	}
}

func TestOnlyAMintedIdentifierCounts(t *testing.T) {
	// A reference to something shaped differently is a broken link in a
	// document, not a row to go looking for.
	for _, bad := range []string{"attachment:short", "attachment:" + strings.ToUpper(token),
		"attachment:../../etc/passwd", "attachment:"} {
		if got := markdown.References("[x](" + bad + ")"); len(got) != 0 {
			t.Errorf("%q resolved to %v", bad, got)
		}
	}
}

func TestMentionsAreReadFromProseAndNotFromCode(t *testing.T) {
	// NTF-12 tells whoever was named, so what counts as naming somebody has to
	// be narrow: a log line pasted into a justification has not called for
	// anybody, and being told you were mentioned when you were not is how
	// people learn to ignore the thing.
	for _, c := range []struct {
		what   string
		source string
		want   []string
	}{
		{"a plain mention", "Asking @ana about this.", []string{"ana"}},
		{"two, once each", "@ana and @ben and @ana again", []string{"ana", "ben"}},
		{"one at the start", "@ana asked.", []string{"ana"}},
		{"inside a code span", "Write it as `@ana` to call somebody", nil},
		{"inside a fenced block", "```\nfrom @ana to @ben\n```\n", nil},
		{"an email address", "Mail ops@example.com about it", nil},
		{"trailing punctuation is not part of the name", "Thanks @ana.", []string{"ana"}},
	} {
		t.Run(c.what, func(t *testing.T) {
			got := markdown.Mentions(c.source)
			if len(got) != len(c.want) {
				t.Fatalf("read %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("read %v, want %v", got, c.want)
				}
			}
		})
	}
}
