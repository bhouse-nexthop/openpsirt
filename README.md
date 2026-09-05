<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/openpsirt-logo-dark.svg">
  <img alt="OpenPSIRT" src="assets/openpsirt-logo.svg" width="380">
</picture>

Track vulnerabilities in the products you ship.

OpenPSIRT takes in the inventory a build produced, scans it for known
vulnerabilities, works out what changed release to release, and gives people a
place to triage what it finds and track it through to a fix.

> **Status: early development.** Not released, and nothing is compatible with
> anything yet — a schema change edits the migration that created it, and a
> development database is recreated rather than migrated.
>
> What exists: the build and validation pipeline; the database layer across all
> four supported engines; the catalog and dependency graph; inventory upload
> and the reader behind it; scanning, run here, with findings tracked over
> intervals and everything tracked scanned again on a schedule; sign-in through
> OIDC, GitHub or a trusted header, with sessions, API keys and personal
> tokens; roles and visibility enforced in the data layer; and triage —
> decisions, approval, revision history, comments, bulk claims and the review
> queue.
>
> Reporting: release-to-release comparison, trends, deadlines, release
> readiness and what a new line would inherit. A fix is declared rather than
> completed — somebody says which releases it is meant to reach, and the next
> scan of each answers whether it arrived. What people are told about is an
> area inside the application, and mail now carries it out of one: the
> categories worth interrupting somebody for go immediately, and a daily
> digest — off until asked for — carries what nothing else said. A message
> about a finding nobody has announced says only that there is something.
>
> Private findings have begun, which is where the work is. A flaw in what a
> build ships can be recorded by hand, from the findings list of the build it
> is in; it starts undisclosed, its embargo has an end, moving that end costs a
> reason and past a threshold a second person, and the date arriving tells
> somebody.
>
> The web interface: sign-in, home, the catalog, findings, finding detail, the
> dependency tree, decisions with their history, the review queue, assignment,
> bulk triage, inventory upload, release comparison, people and roles, and
> settings — embedded into the binary and served from it.
>
> An advisory about such a flaw is generated as a CSAF document from what is
> already held — and generated is all: nothing is sent anywhere, because the
> triage record is ours and a published advisory belongs to whoever publishes
> it.
>
> What does not: every adapter that would send an advisory somewhere,
> attachments, chat, remediation metrics, export, and findings from a static
> analyzer. The design record is in [DECISIONS.md](DECISIONS.md).

## What it does

- **Takes in inventories** pushed by build pipelines, in CycloneDX form, along
  with the suppressions the build carries patches for
- **Runs the scan here**, not in the build, so every product is measured
  against the same scanner and the same vulnerability data
- **Keeps the dependency graph**, so you can see *why* a vulnerable component is
  present and which part of the product pulled it in
- **Tracks change over time** per release, and works out why a finding
  disappeared rather than guessing
- **Carries triage decisions forward**, so a nightly scan doesn't reset the work
- **Rescans shipped releases**, so a CVE published after a release still gets
  found
- **Reports** on what was fixed between releases, what was dismissed and why,
  what is running out of time, and whether the team is keeping pace

## What it doesn't do

- Generate SBOMs — your build does that
- Discover what is in a product — the component list always comes from the build
- Build or deploy fixes

## Trying it

    make demo                    # build the image, start it, seed two products, print the address
    make demo DEMO_HOST=yourbox  # if you browse by something other than localhost

It seeds two products: a real switch image, and OpenPSIRT itself, from the
inventory the image carries of what it ships — so the screens that compare
across products have something to compare.

**Docker is all you need to build and run it**, plus `curl` and `xz`, which
seed it from the compressed fixtures and are already on most machines. The
image builds the interface and the binary inside itself and carries the
scanner, so there is nothing else to set up first — and it builds from your
working tree, so what comes up is your change.

**One person cannot demonstrate this tool.** A judgment is proposed by one
person and agreed to by another, and approving your own is refused — so a
single identity can propose a decision and never finish one. The demo therefore
opens a door per person: `http://localhost:8080` arrives as an administrator,
`:8081` as Ana and `:8082` as Ben, both triagers who may approve. Two browser
windows are two people. Change or extend the cast with `DEMO_CAST`, which takes
`port:name:roles` entries:

    make demo DEMO_CAST="8091:ana:public-read,public-triage,approver \
                         8092:ben:public-read,public-triage,approver"

`make demo-status` says what it found and lists every door, `make demo-down`
stops it, and `make demo-reset` starts over while keeping the scanner's
vulnerability database, which is large and slow to fetch. Everything it writes
stays in a git-ignored directory in the checkout.

`make dev` is the other one: this machine's binary plus the interface's own dev
server, for editing the interface and watching it reload. It needs Go, node and
a scanner installed locally, and it does not exercise the interface the binary
embeds — `make demo` does.

It is a demonstration deployment rather than a small production one — plain
HTTP, and administration handed to whoever the proxy in front of it says they
are. `DESIGN-interface.md` says what that costs.

## Documentation

| | |
|---|---|
| [DECISIONS.md](DECISIONS.md) | Every decision, with reasoning, organized by area |
| [AGENTS.md](AGENTS.md) | Conventions for anyone, human or otherwise, working in this repository |
| [docs/](docs/) | What is published: installing, configuring and the API reference |

`DESIGN-*.md` documents describe how each area actually works, and appear as
each is built:

| | |
|---|---|
| [DESIGN-access.md](DESIGN-access.md) | Who is asking, and what they may reach |
| [DESIGN-api.md](DESIGN-api.md) | The shape of the HTTP surface |
| [DESIGN-attachments.md](DESIGN-attachments.md) | Files on a finding — the seam that is built, and the rest that is not |
| [DESIGN-build.md](DESIGN-build.md) | Layout, the validation pipeline, how a change is checked |
| [DESIGN-data-model.md](DESIGN-data-model.md) | What a scan is filed against, and the dependency graph |
| [DESIGN-database.md](DESIGN-database.md) | Four engines, migrations, locking |
| [DESIGN-findings.md](DESIGN-findings.md) | What a scan run found, and where |
| [DESIGN-ingest.md](DESIGN-ingest.md) | What happens to a scan when it arrives, and how one is read |
| [DESIGN-interface.md](DESIGN-interface.md) | The web interface, how it is built and how it reaches the server |
| [DESIGN-notifications.md](DESIGN-notifications.md) | What people are told about, and what they are not |
| [DESIGN-packaging.md](DESIGN-packaging.md) | Container image and Helm chart |
| [DESIGN-queue.md](DESIGN-queue.md) | How work waiting to be done is held and picked up |
| [DESIGN-remediation.md](DESIGN-remediation.md) | Which releases a fix is meant to reach, and how the scans answer |
| [DESIGN-reporting.md](DESIGN-reporting.md) | Trends, release comparison, deadlines, settings |
| [DESIGN-text.md](DESIGN-text.md) | What may be written, and how it is rendered |
| [DESIGN-triage.md](DESIGN-triage.md) | What people decide about findings, and when a decision stops applying |

## License

Apache 2.0. See [LICENSE](LICENSE).
