# Reader fixtures

What each file is, and where it came from. A fixture nobody can trace is one
nobody can tell a producer quirk from a typo in.

| File | Origin |
|---|---|
| `gomod-app.cdx.json` | Real output. This project's own inventory, emitted by the Go module producer. Its identifiers are not its package identifiers, and it states most of its structure by nesting rather than by naming edges — both of which a reader written against one producer gets wrong |
| `build-fragment.cdx.json` | Real output. One artifact a build step produced, on its way into an inventory. It names no component of its own, which is why it is refused |
| `suppression-from-patch.openvex.json` | Real output. One claim a build extracted from a patch of its own. It names a source tree rather than a package, which is what makes matching a claim to a component something that can fail |
| `image.cdx.json` | Written by hand, in the shape of the aggregate inventory a switch operating-system build emits: an image at the root, containers under it, packages under those, a shared library reached from several of them, and a forked component whose pedigree carries the version it was forked from. Not a producer's output, and not a substitute for one |

`producer-paths.txt` records every key path these documents contain and what
the reader does with it. Regenerate with `go test ./internal/sbom -update`,
which adds paths it has not seen before as deliberately skipped and leaves
every decision already made alone.
