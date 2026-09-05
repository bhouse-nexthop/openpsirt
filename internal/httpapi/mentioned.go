package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
	"github.com/bhouse-nexthop/openpsirt/internal/notify"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// mentionCap bounds how many people one piece of text can call for.
//
// A justification naming forty people is not a question for any of them, and
// without a bound one comment is one write per person. Generous enough that
// nobody hits it while writing normally.
const mentionCap = 20

// tellMentioned tells whoever a decision's new text named (NTF-12).
//
// The decision is read back rather than carried out of the write, because what
// the notification needs — which product, which issue, how disclosed — is what
// the reader is authorized against, and reading it through the same store the
// write went through means one answer to "may this person reach it".
//
// Failing to tell somebody never fails the write. The words are on record by
// the time this runs, and losing a comment because a notification could not be
// stored would be sacrificing the wrong half.
func tellMentioned(ctx context.Context, in Ingest, subject access.Subject,
	store *triage.Store, decisionID int64, body string) {

	if len(markdown.Mentions(body)) == 0 {
		return
	}
	decision, _, err := store.Read(ctx, subject, decisionID)
	if err != nil {
		in.Logger.WarnContext(ctx, "could not tell who was named", "error", err)
		return
	}
	if err := mentioned(ctx, in, subject, decision, body,
		fmt.Sprintf("/decisions/%d", decisionID)); err != nil {
		in.Logger.WarnContext(ctx, "could not tell who was named", "error", err)
	}
}

// mentioned tells whoever a piece of text named that it named them (NTF-12).
//
// **Only people who could already read it.** The set is exactly the set the
// editor offers, from the same query, so a mention cannot tell somebody that a
// finding exists when they may not see it — on an undisclosed one the
// notification itself would be the disclosure.
//
// **Never the author.** Somebody writing their own name is not asking
// themselves a question, and a tool that tells you what you just typed is one
// people stop reading.
//
// Failing to tell somebody is not failing to save the text. The words are on
// record by the time this runs, and losing a comment because a notification
// could not be written would be the wrong half to sacrifice — so this reports
// and the caller logs.
func mentioned(ctx context.Context, in Ingest, subject access.Subject,
	decision *triage.Decision, body, link string) error {

	names := markdown.Mentions(body)
	if len(names) == 0 || decision == nil {
		return nil
	}
	if len(names) > mentionCap {
		names = names[:mentionCap]
	}

	// Who may read this, at the visibility of the thing the text is about.
	// Asked of the same rule the editor's list uses rather than spelled again
	// here, so the two cannot come to disagree about who may be named.
	readers, err := access.NewStore(in.DB.DB).WhoCanRead(ctx,
		decision.ProductID, decision.Visibility, 100)
	if err != nil {
		return fmt.Errorf("read who may be told: %w", err)
	}
	byName := make(map[string]int64, len(readers))
	for _, reader := range readers {
		byName[strings.ToLower(reader.Identity)] = reader.ID
	}

	told := map[int64]bool{subject.ID: true}
	for _, name := range names {
		who, known := byName[strings.ToLower(name)]
		// A name nobody holds, and a name held by somebody who may not read
		// this, are both simply not told — and they are not told apart. A
		// refusal naming which it was would answer, one comment at a time,
		// whether a given person can see undisclosed work.
		if !known || told[who] {
			continue
		}
		told[who] = true
		if err := notify.NewStore(in.DB.DB).Tell(ctx, notify.Telling{
			PersonID: who, Kind: notify.Mentioned,
			Body: fmt.Sprintf("%s named you in a note on %s.",
				whoever(subject), decision.PlaceIdentity[:min(8, len(decision.PlaceIdentity))]),
			Link:     link,
			Private:  decision.Visibility == access.Private,
			Concerns: notify.Concerning(decision.ProductID, decision.VulnerabilityID, 0),
		}); err != nil {
			return fmt.Errorf("tell %d they were named: %w", who, err)
		}
	}
	return nil
}

// whoever is what to call the person who wrote the text.
func whoever(subject access.Subject) string {
	if name := strings.TrimSpace(subject.Identity); name != "" {
		return name
	}
	return "Somebody"
}
