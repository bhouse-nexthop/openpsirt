# Text people write

Justifications, deferral reasons and comments, and the one place typed text
becomes markup.

Satisfies TRI-28, SEC-04, SEC-11 to SEC-19, API-16 to API-18.

## Two halves, kept apart

**Policy runs once, at submission, before anything is stored.** What is
permitted, which links survive, and what a reference resolves to are decided
there — because that is the half that cannot be duplicated. It is the security
control, and it needs data and authorization checks no client holds.

**Rendering runs on the way out, every time.** What is stored is the source and
never the markup. A sanitizer fixed next year does nothing for markup already
sitting in a database, and the same text has to reach a browser, an email and
an export — three renders from one source.

Both run. Refusing at submission is what tells somebody their text will not do
what they meant; sanitizing at render is what covers text stored before a rule
existed. Neither replaces the other.

## Raw markup is refused at the parser

Not stripped afterwards. An allowlist of permitted tags is a thing that can be
wrong, and every interesting attack lives in the gap between what such a list
permits and what a browser actually does with it. Refusing the feature removes
the category, and nothing anybody needs for triage requires it.

The parser drops a raw block outright and escapes an inline one. Both are safe,
and which happens depends on where it appeared — so what is checked is that
nothing arrives as live markup, rather than which of the two mechanisms ran.

The sanitizer runs over the output anyway. That is checked separately, because
with the sanitizer in front, turning the parser's guard off changes nothing
observable — and a layer whose absence is invisible is a layer that will
eventually be absent.

## Links are permitted, and their schemes are not

A policy that strips every link is safe and useless. An advisory link is the
most common thing in a justification, and one that silently became plain text
would send somebody back to a search engine — which is the problem the evidence
was there to solve.

So `http`, `https` and `mailto` survive and everything else is dropped.
`javascript:` in a link is the oldest attack there is, and a `data:` address
lets a link become a page we appear to have served. Outbound links carry
nothing back: no referrer, no live opener.

This was nearly lost without anybody noticing. Breaking the scheme restriction
deliberately changed nothing, because links had never been permitted at all —
every one had been quietly becoming plain text, and the restriction was
guarding a door that was already bricked up.

**A link to somewhere in this deployment survives too.** One finding referring
to another is ordinary, and the submission check says so and accepts it — while
the sanitizer went on deleting the anchor and leaving the text behind. Two
halves of one rule disagreeing is worse than either answer alone, because
nothing reports it: a link accepted when it was written stopped being a link
when anybody read it.

## Markup is a representation, offered on request

What comes back by default is markdown. It is what an integrating application
can most easily lay out, and it reads as plain text as it stands, so it doubles
as the plain form. HTML assumes a browser, which most callers of an API-first
tool are not — so a caller that wants markup asks for it.

Markup is produced on the way out, every time, and never stored. A sanitizer
rule written next year then applies to text written last year, which it could
not if markup had been stored when the text arrived. That is also why the
sanitizer runs on render at all, when the submission check already refused the
text: stored text predates rules written since, and a control that only ran
when the text arrived protects nothing written before it existed. There is a
test that writes a payload straight into the row, past the submission check, to
prove the second half runs.

A rendering that fails yields nothing rather than failing the request. The
source is in the same answer and is the authoritative form; refusing to return
a decision because one field could not be turned into markup would make a
presentation problem into an outage.

## Nothing is fetched

No image is rendered, from anywhere. Not restricted by address — the element is
not permitted at all, because the address is not the problem.

An image fires from the browser of everybody who reads the text, from inside
the network, telling whoever wrote it who is looking at which finding and when.
On an undisclosed finding that is a disclosure channel. Files are attached
instead, and fetched through a path that checks who is asking.

## What a scan file said is shown, never rendered

Once a renderer exists, pointing it at a component description is the obvious
next step — and that hands whoever wrote the scan file a formatting language
aimed at the browsers of the people who hold the most access here.

So text from a scan file is escaped and displayed as written, whatever it looks
like.

## A refusal says where to look

A justification is forty lines, and "remote images are not allowed" means
hunting. Somebody who cannot find what to fix rewrites the whole thing or stops
explaining themselves, and an explanation nobody writes is the thing this
system exists to collect.

So a refusal carries the line, the text that caused it, and why. **Everything
wrong is reported at once**: fixing one problem and resubmitting to find the
next is how a person learns to write plain sentences with no evidence in them.

## The language tag on a code block is input

Three backticks followed by attacker-chosen text landing in a class attribute
is small and real, and it is the only highlighting-related hole left once the
highlighter runs over already-sanitized markup rather than before it.

The tag is allowlisted. An unrecognized language keeps the block and loses the
label, rather than failing — refusing a justification over a language nobody
listed would make the tool argue with people about syntax highlighting.

**The allowlist is applied in one place**, by the sanitizer, on the way out. A
second check on the way in looked like defense in depth and was two spellings
of one rule; two spellings diverge, and then the tag a submission accepted is
not the tag the sanitizer keeps. What is asserted is the rendered output rather
than the allowlist, because asking the allowlist directly proves only that it
agrees with itself.

## Bounds

Every field is capped at 64 KB and rendering is time-bounded. Rendering is work
somebody else asked us to do, what is stored is kept forever under an
append-only rule, and a pathological input should fail one request rather than
hold one open indefinitely.

**The time bound bounds the wait, not the work.** A parse cannot be
interrupted, so the work runs to completion with nobody reading the result.
What the bound buys is that the request answers and its resources are released.
The cap on the work itself is the length limit, applied before any of it starts
— which is why the 64 KB is a control rather than tidiness.

## Choices the decisions did not cover

- **What is checked as an address.** A bare `scheme://` is treated as a link;
  a bare `word:` is not, because the second matches `parser.go:112` in a stack
  trace and `TODO: check this` in a sentence. A policy that argues with a stack
  trace is one people route around. The schemes that *do* something rather than
  go somewhere are matched separately, since they carry no `//`.
- **A tag in the text is reported rather than silently dropped.** Raw markup is
  already off at the parser, so this changes nothing about safety — it is so
  that somebody who wrote a tag is told why it will not appear, instead of
  watching it vanish.
