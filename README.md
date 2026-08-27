# openpsirt

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

## Design

Every decision, with the reasoning, is in [DECISIONS.md](DECISIONS.md). It is
organised by area and each entry says why, not only what.

## Licence

Apache 2.0. See [LICENSE](LICENSE).
