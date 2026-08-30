# Reader fixtures

What each file is, and where it came from. A fixture nobody can trace is one
nobody can tell a producer quirk from a typo in.

| File | Origin |
|---|---|
| `gomod-app.cdx.json` | Real output. This project's own inventory, emitted by the Go module producer. Its identifiers are not its package identifiers, and it states most of its structure by nesting rather than by naming edges — both of which a reader written against one producer gets wrong |
| `build-fragment.cdx.json` | Real output. One artifact a build step produced, on its way into an inventory. It names no component of its own, which is why it is refused |
| `suppression-from-patch.openvex.json` | Real output. One claim a build extracted from a patch of its own. It names a source tree rather than a package, which is what makes matching a claim to a component something that can fail |
| `image.cdx.json` | Written by hand, in the shape of the aggregate inventory a switch operating-system build emits: an image at the root, containers under it, packages under those, a shared library reached from several of them, and a forked component whose pedigree carries the version it was forked from. Not a producer's output, and not a substitute for one |
| `switch-image.cdx.json.xz` | Real output, and the full-size one. The public SONiC network operating-system image, 8,374 components and 25,123 edges over 20 MB, from an upstream build. Compressed because the shape is the point and 20 MB of it is not. See below |

`producer-paths.txt` records every key path these documents contain and what
the reader does with it. Regenerate with `go test ./internal/sbom -update`,
which adds paths it has not seen before as deliberately skipped and leaves
every decision already made alone.


## The full-size fixture

`switch-image.cdx.json.xz` is what a real aggregate inventory looks like, and
it is here because a hand-written one cannot stand in for it. Two things it
proves that nothing written on purpose would:

**It contains the same package twice, 516 times over.** The build merges two
sources without reconciling them, so a package arrives once with a platform
identifier, an upstream qualifier and a vulnerability-database identifier, and
once with only an architecture. `acl@2.3.2-2+b1` is both
`...?arch=amd64&distro=debian-13&package-id=7adffac816f3efd6&upstream=acl%402.3.2-2`
and `...?arch=amd64`. One of the two carries what a scanner needs to match it;
the other does not.

**It spells the same package identifier two ways.** One of that pair escapes
the `+` in a version and the other leaves it, so byte comparison says they are
different packages and they are not.

Both are real producer behavior rather than anything anybody would think to
write down, and both are exactly what deriving identity from content rather
than from what a file says is meant to survive.

It is stored compressed. The uncompressed document is 20 MB, which is a size
worth reading once in a test and not a size worth keeping in every checkout.
