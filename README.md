<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/openpsirt-logo-dark.svg">
  <img alt="OpenPSIRT" src="assets/openpsirt-logo.svg" width="380">
</picture>

Track vulnerabilities in the products you ship.

openpsirt takes in SBOMs that already carry vulnerability data, works out what
changed release to release, and gives people a place to triage what it finds and
track it through to a fix.

> **Status: design.** There is no code yet. This repository currently holds the
> design record — see [DECISIONS.md](DECISIONS.md).

## What it does

- **Takes in scan results** pushed by build pipelines, in CycloneDX form
- **Keeps the dependency graph**, so you can see *why* a vulnerable component is
  present and which part of the product pulled it in
- **Tracks change over time** per release, and works out why a finding
  disappeared rather than guessing
- **Carries triage decisions forward**, so a nightly scan doesn't reset the work
- **Rescans shipped releases**, so a CVE published after a release still gets
  found
- **Reports** on what was fixed between releases, what was dismissed and why, and
  whether the team is keeping pace

## What it doesn't do

- Generate SBOMs — your build does that
- Discover what is in a product — the component list always comes from the build
- Build or deploy fixes

## Documentation

| | |
|---|---|
| [DECISIONS.md](DECISIONS.md) | Every decision, with reasoning, organised by area |
| [IMPLEMENTATION.md](IMPLEMENTATION.md) | Build order. Temporary — deleted once the work lands |
| [AGENTS.md](AGENTS.md) | Conventions for anyone, human or otherwise, working in this repository |

`DESIGN-*.md` documents appear as each area is built, and describe how it
actually works.

## Licence

Apache 2.0. See [LICENSE](LICENSE).
