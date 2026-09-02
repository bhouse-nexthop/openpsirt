# Reader fixtures

What each file is, and where it came from. A fixture nobody can trace is one
nobody can tell a producer quirk from a typo in.

| File | Origin |
|---|---|
| `gomod-app.cdx.json` | Real output. This project's own inventory, emitted by the Go module producer. Its identifiers are not its package identifiers, and it states most of its structure by nesting rather than by naming edges — both of which a reader written against one producer gets wrong |
| `build-fragment.cdx.json` | Real output. One artifact a build step produced, on its way into an inventory. It names no component of its own, which is why it is refused |
| `suppression-from-patch.openvex.json` | Real output. One claim a build extracted from a patch of its own. It names a source tree rather than a package, which is what makes matching a claim to a component something that can fail |
| `image.cdx.json` | Written by hand, in the shape of the aggregate inventory a switch operating-system build emits: an image at the root, containers under it, packages under those, a shared library reached from several of them, and a forked component whose pedigree carries the version it was forked from. Not a producer's output, and not a substitute for one |
| `switch-image.cdx.json.xz` | Real output, and the full-size one. The public SONiC network operating-system image, 6,845 components and 18,561 edges over 17 MB, from a build of sonic-buildimage#29237. Compressed because the shape is the point and 18 MB of it is not. See below |

`producer-paths.txt` records every key path these documents contain and what
the reader does with it. Regenerate with `go test ./internal/sbom -update`,
which adds paths it has not seen before as deliberately skipped and leaves
every decision already made alone.


## The full-size fixture

`switch-image.cdx.json.xz` is what a real aggregate inventory looks like, and
it is here because a hand-written one cannot stand in for it. What it holds
that nothing written on purpose would:

**A graph rather than a tree.** 1,095 of its components have more than one
direct consumer, which is what makes "why is this here" a question with more
than one answer, and what a reader assuming a tree gets wrong.

**A hierarchy rather than a root with everything under it.** The image root has
30 direct children — 29 containers and the host filesystem — and the packages
installed on the host hang off the host rather than off the image. The previous
build gave the root 5,198 direct children and left 237 components with no
consumer at all, which is a shape no reader can answer "why is this here" from.
39 are still unreached, and they are lockfile and recipe fragments the build
emits without saying what consumed them.

**Build tooling kept apart from what ships.** 849 components sit under
`formulation` — Go and Rust dependencies harvested from inside the build
containers — rather than in `components` beside the image's contents. The
question a scanner answers is what shipped; the question a build-chain
compromise asks is what built it, and the document now answers both without
either being mistaken for the other.

**What it no longer holds is worth writing down**, because this file used to be
the evidence for a rule and is not any more. It carried the same package twice,
516 times over, spelled two ways — once with a platform identifier and an
upstream qualifier, once with only an architecture, and sometimes escaping the
`+` in a version and sometimes not. That was one producer's merge step, fixed
upstream in sonic-buildimage #29237, and this fixture now arrives as one
component per package. **So it no longer exercises the merging rule it was
originally kept for** — deleting that code entirely leaves the full-size test
passing. The rule is proved by a test that constructs the duplicates instead,
which is where a rule of this kind belongs: a fixture is somebody else's output
and can stop exercising a rule without anybody deciding it should.

It is stored compressed. The uncompressed document is 18 MB, which is a size
worth reading once in a test and not a size worth keeping in every checkout.
