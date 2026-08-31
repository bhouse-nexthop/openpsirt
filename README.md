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
> intervals; sign-in through OIDC, GitHub or a trusted header, with sessions,
> API keys and personal tokens; roles and visibility enforced in the data
> layer; and triage — decisions, approval, revision history, comments and the
> review queue.
>
> Reporting has begun: release-to-release comparison, trends, deadlines and
> what a new line would inherit.
>
> What does not: the web interface, remediation tracking, notifications,
> private findings and attachments. The design record is in
> [DECISIONS.md](DECISIONS.md).

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

## Documentation

| | |
|---|---|
| [DECISIONS.md](DECISIONS.md) | Every decision, with reasoning, organized by area |
| [IMPLEMENTATION.md](IMPLEMENTATION.md) | Build order. Temporary — deleted once the work lands |
| [AGENTS.md](AGENTS.md) | Conventions for anyone, human or otherwise, working in this repository |

`DESIGN-*.md` documents describe how each area actually works, and appear as
each is built:

| | |
|---|---|
| [DESIGN-access.md](DESIGN-access.md) | Who is asking, and what they may reach |
| [DESIGN-api.md](DESIGN-api.md) | The shape of the HTTP surface |
| [DESIGN-build.md](DESIGN-build.md) | Layout, the validation pipeline, how a change is checked |
| [DESIGN-data-model.md](DESIGN-data-model.md) | What a scan is filed against, and the dependency graph |
| [DESIGN-database.md](DESIGN-database.md) | Four engines, migrations, locking |
| [DESIGN-findings.md](DESIGN-findings.md) | What a scan run found, and where |
| [DESIGN-ingest.md](DESIGN-ingest.md) | What happens to a scan when it arrives, and how one is read |
| [DESIGN-packaging.md](DESIGN-packaging.md) | Container image and Helm chart |
| [DESIGN-queue.md](DESIGN-queue.md) | How work waiting to be done is held and picked up |
| [DESIGN-reporting.md](DESIGN-reporting.md) | Trends, release comparison, deadlines, settings |
| [DESIGN-text.md](DESIGN-text.md) | What may be written, and how it is rendered |
| [DESIGN-triage.md](DESIGN-triage.md) | What people decide about findings, and when a decision stops applying |

## License

Apache 2.0. See [LICENSE](LICENSE).
