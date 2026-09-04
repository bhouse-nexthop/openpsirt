# Packaging and deployment

How OpenPSIRT is delivered, and what a deployment looks like.

Satisfies SCP-03, SCP-04, SCP-08, SCP-09, SCP-12 to SCP-14, SCP-16, and the
probe behavior DAT-10 requires.

## The image

Multi-stage: the interface built with node, the binary built with the Go
toolchain the module asks for, and the result run on Alpine.

**The interface is built here rather than taken from the build context.** The
Go build embeds a directory that is git-ignored, so an image built from a clean
checkout — which is what CI builds — carried no interface at all, and nothing
said so: an API-only binary is a supported build and the embed tolerates an
empty directory on purpose. Building it in the image also means the image needs
nothing of the builder's machine but docker, which is what makes standing an
instance up one command.

The binary is **fully static** — CGO is off, which is part of why the pure-Go
SQLite driver was chosen. That means the runtime image could be `scratch` or
distroless and carry no shell at all.

Alpine is used anyway, deliberately. A self-hosted operator debugging their own
deployment wants a shell, and that is worth the few megabytes and the extra
surface. It is one line to change if that trade ever stops being worth it.

**The version is pinned in one place, and the pin has an expiry somebody has to
watch.** Every stage that is not somebody else's toolchain image starts from
the same argument, so three of them cannot drift apart. It is pinned rather
than tracking latest for the reason the scanner is: what a finding means
depends on what was measured, and a base that moves under a rebuild changes the
answer without anybody asking it to.

What that costs is that nothing moves it either. The image carried 3.21 —
released December 2024, support ending 2026-11-01 — until somebody looked, by
which point it was three releases behind and weeks from the date. **Past a
base's end-of-life its packages get no security backports**, so the seventeen
Alpine packages this image ships would accumulate findings with no fix
available, and this tool would report them against its own image and be right.
That is the shape of the failure rather than a one-off: the same week found the
scanner and the inventory tool two releases behind each, and nothing watches
any of the three.

**The packages inside that release are upgraded at build time** (SCP-17), which
is a different thing from moving the pin and does not weaken it. A base tag's
package set is frozen at the moment that image was published and the
distribution goes on publishing fixes for it, so pinning the base and stopping
there ships what was known-vulnerable on that date and keeps shipping it.

Measured on ourselves the day the base moved: **22 findings against the
distribution's own packages, 20 of them OpenSSL at `3.5.7-r0` with the fix
published as `3.5.8-r0`**, two of them critical, every one matched through
Alpine's own advisories rather than by comparing an identifier against an
upstream range. Upgrading cleared them. They were not caused by the newer base;
the older one had the same shape and was less legible about it.

What this costs is that two builds of one commit can differ, and that is
accepted rather than overlooked. It is acceptable here for a reason particular
to this image: it carries an inventory of itself, so what actually shipped is
recorded rather than assumed, and the drift is something somebody can read
instead of something nobody can see.

| | |
|---|---|
| Size | ~40 MB |
| User | Non-root, no login shell, no home directory |
| Extras | The vulnerability scanner. Root certificates, for outbound TLS to the ranking feeds and to the scanner's data. Timezone data |
| Healthcheck | Liveness only — readiness needs the database and belongs to the orchestrator, which can act on it |

CI builds the image, runs it, **checks it is not running as root**, and
**checks the scanner in it actually runs**. Those are the only places those
claims are tested rather than asserted.

It also **checks the image serves the interface**, which is the check whose
absence let an image with no interface in it pass for as long as it did. An
image built without one answers the page's own path with a credential refusal
rather than a page, so the difference is visible from outside.

### The scanner ships with it

This deployment runs the scan rather than trusting whatever a producer's
pipeline happened to install, so an image without a scanner takes in
inventories it can never answer a question about. It is pinned to a version and
its download is checksummed: which build of a scanner answered is part of what
a finding means, because counts are only comparable between products measured
the same way.

Its **vulnerability data is fetched at runtime, not built in**. A database
baked into an image is stale the day after that image is published, and the
whole reason the scan happens here is that this data moves under an inventory
that does not.

That data needs somewhere writable, and the root filesystem is read-only — so
the chart mounts a volume for it. The default is scratch space that lives as
long as the pod, which is correct and re-downloads on every start. A deployment
that restarts often, or would rather not fetch that much each time, points it
at a claim instead.

The image and the chart name the same directory. Changing it in one and not the
other puts the data back on the read-only filesystem, where it cannot be
written — so both read from one value in the chart, and the image sets a
default that matches.

## The chart

Under SCP-03 the people running this are operators we never meet, so the chart
is where the things that are easy to get wrong are got right once.

### Probes

Three, answering three different questions:

| Probe | Asks | Path |
|---|---|---|
| Startup | Has it finished starting? | `/readyz` |
| Liveness | Is the process running? | `/healthz` |
| Readiness | Can it serve? | `/readyz` |

**Liveness must not depend on the database.** Restarting a process cannot fix
an unreachable database, and a liveness probe that fails on it turns a database
outage into a restart loop on top of a database outage.

**The startup probe allows ten minutes by default.** Migrations run before the
service answers, and a schema change over a large table can take a while. A
probe that gives up part way through kills the pod and starts the migration
again from the beginning — turning a slow upgrade into an outage. The default is
`failureThreshold` × `periodSeconds`, and both are settable.

### Security context

Non-root, read-only root filesystem, every capability dropped, and the default
seccomp profile. Nothing here needs any of them. `/tmp` is an `emptyDir` with
a size limit, because a read-only root still needs somewhere to put a scratch
file and an unbounded scratch directory is node disk that a stray file can
fill.

The pod's termination grace is twice the process's own shutdown grace: the
process drains requests for that long and then waits the same again for a
worker mid-scan, so a shorter grace kills a scan being applied and leaves it
for the queue to retry.

**A SQLite URL with more than one replica is refused at render.** Every
replica serves, reads and scans, coordinated through the database (SCP-15),
and SQLite is a file inside one pod — a second replica would start with an
empty database of its own. The chart can only see the URL when it is given in
values; a URL in a Secret of the operator's own is not checked here, and the
process warns at startup that SQLite is not a production engine.

The service account has `automountServiceAccountToken: false` — OpenPSIRT never
talks to the Kubernetes API, so it has no use for a token that would otherwise
sit in every pod.

### The database URL

From a Secret, always. Either name one you manage, or let the chart create one
from a value — with the caveat stated in the values file that a password put
there ends up in your release history and probably in version control.

Setting both, or neither, **fails at render time with a sentence naming the
problem**. Rendering manifests that cannot work would move the failure to a
crash-looping pod and a message nobody reads.

### Who may sign in

The chart carries the sign-in configuration, and **refuses to render an install
that could not be signed into**. Four ways to get that wrong, each failing at
render time with a sentence naming the problem:

| | |
|---|---|
| No bootstrap admin | Nobody can administer the deployment |
| No provider | Naming a bootstrap admin grants a role; it does not let anybody in |
| A provider with no base URL | A provider returns somebody to an address and compares it against what it was registered with |
| A trusted header with no sources | A header named with nothing to trust it from is a header anybody can set |

This is the same reasoning as the database URL, applied to the other thing that
makes an install unusable. Without it the chart rendered happily, the process
started, failed its own administration check, and crash-looped with the reason
in a log nobody is watching during an install.

The refusals are tested by asserting each one fires, not by asserting a good
install renders. A guard nobody has watched refuse is not a guard.

Naming bootstrap admins is also the **recovery path**, which is why they are
applied at every startup rather than once: lose administrative access, add
yourself, upgrade. The notes printed after an install say so, because that is
the moment somebody will need it and the last moment they will be reading.

## What is not here yet

Publishing. The image builds and is verified in CI but is not pushed anywhere,
and the chart is not published to a repository. Both belong with a release
process, which does not exist yet — so the chart's default image reference
points at something that does not exist, and the values file says so rather
than leaving an operator to discover it as a pod that cannot pull.

## Apache 2.0, and what that means for delivery

The licence is Apache 2.0 (SCP-04), which is a delivery decision as much as a
legal one: whoever runs this compiled it or pulled it, and can read and change
what they run. Two things follow that this document has to keep true.

**Everything needed to run it is in the image or is an ordinary dependency.**
No component is fetched at run time from somewhere only we can reach, and
nothing is gated on a key we issue. An operator who mirrors the image into
their own registry has the whole thing.

**The image says what it is made of**, which is the section below: a tool whose
subject is knowing what is in a build has no standing to ship an opaque one.

## The image carries two inventories, and they are not the same list

**What the binary was linked from**, at `/usr/share/openpsirt/openpsirt.cdx.json`
(SCP-08, SCP-09). Generated in the build stage by reading the built binary: the
build information it carries names every module and the main module's version,
so no checkout is needed and the build context carries none. The generator's
version is pinned by the same variable the `sbom` target uses, so the image and
the release asset are made the same way.

**What the image ships**, at `/usr/share/openpsirt/image.cdx.json`. The first
inventory describes a program; this describes a container. musl, busybox, the
certificate bundle and the scanner that rides along are all shipped here and
appear in none of the first one — and for a tool whose entire subject is
knowing what is inside what you ship, carrying only the half that leaves out
most of the image is the wrong half to carry alone.

It is read off the assembled filesystem rather than by scanning an image,
because the image being described does not exist until the build finishes. The
runtime is a stage; a later stage copies that stage's filesystem and catalogs
it. What is described is therefore this build rather than one like it.

**Packages, not files.** The file catalogers add a component per path with no
version and no package identifier — eight hundred of them here. Nothing
downstream can act on those: a scanner matches packages, so they would be eight
hundred rows in a dependency tree that no finding can ever hang off. They also
carry the scan path, which is a build-time detail with no business in a shipped
inventory. With them off the answer is 357 components — seventeen Alpine
packages, the operating system itself, and the modules of both binaries.

The generator is pinned by version and checksum, the same way the scanner is
and from the same project. A build that fetches an unpinned tool over the
network is a build whose output depends on the day it ran.

**It is cataloged as parts and composed**, because neither way of asking
answers the whole question. Cataloging a directory finds every package and
loses the structure inside a compiled binary: the modules arrive flat, with
nothing above them, not even the module that *is* the binary. Cataloging one
binary produces the opposite — a proper graph, and no knowledge of the image
around it.

So the filesystem is cataloged with the binaries left out, each binary is
cataloged on its own, and `internal/tools/compose` joins them. Nothing is
inferred: each input says what it found and how it was arranged, and what is
added is one edge from the image to each component nothing else placed — which
is what "this image contains that" already means.

Measured: before, the root had **no children** and 345 components floated. After,
the root has ten and every module sits under the binary it came out of. That is
the difference between a finding that says a vulnerability is in `containerd`
and one that says it is in the scanner this image bundles.

Two rules the composer holds to. **A component is identified by its package
identifier with the producer's qualifiers cut**, because a module two binaries
both link gets a different reference in each catalog — one component with two
parents is the truth, and two components is a count saying the image ships it
twice and a decision that has to be made twice. And **everything else a
producer recorded is carried through untouched**: licences, hashes, the
properties saying where something was found. Composing rewrites references and
adds one edge; dropping the rest would make this a lossy step in the middle of
an audit trail.

**Two things want an inventory in the image.** A release's inventory should
travel with the artifact it describes rather than only sitting beside it on a
release page. And a deployment can then be its own first product: the demo
declares OpenPSIRT and uploads both files as two variants, `binary` and
`container`, so somebody evaluating the tool sees the difference between "what
we wrote" and "what we ship" on itself, on the first screen they open, without
owning a build pipeline.
