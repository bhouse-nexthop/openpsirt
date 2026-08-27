# Build and validation

How a change gets from an editor to `main`, and what stops it if it should not.

Satisfies CIG-01 to CIG-08, SCP-06, SCP-07, API-04, API-10, API-11.

## Layout

| Path | Holds |
|---|---|
| `cmd/openpsirt/` | The binary. Flag parsing, configuration, server lifecycle |
| `internal/version/` | What this build is. Values injected at link time |
| `internal/config/` | Settings, read from the environment with working defaults |
| `internal/httpapi/` | The HTTP surface and the operations the API document is generated from |
| `docs/` | The published documentation site |
| `assets/` | Logo files |

Everything is under `internal/`, so nothing is importable by another module
until we deliberately decide otherwise.

## One entry point

Every check CI runs is a `make` target. A developer reproduces any CI failure
with the same command and the same pinned tool versions — CIG-06.

| Target | Does |
|---|---|
| `make build` | Builds the binary with version information injected |
| `make test` | Tests with the race detector |
| `make lint` | Static analysis, pinned version |
| `make govulncheck` | Known vulnerabilities in dependencies |
| `make licences` | Licences of shipped dependencies against the allowlist |
| `make openapi` | Regenerates the API document from the code |
| `make check` | Everything above |

## Static analysis

Rule selection lives in `.golangci.yml` and nowhere else — CIG-02. Two scopes
only: a rule gates or it is advisory. There is no third scope for grandfathering
a backlog, because starting from an empty repository there is no backlog to
grandfather — CIG-03.

The linter set is tuned rather than enabled wholesale (CIG-04). Documentation
rules are off; error checking excludes the cleanup-path functions that are
conventionally ignored. A gate that reports mostly style is one somebody turns
off.

**The linter must be built with a Go release at least as new as the code.**
Otherwise it cannot read the compiler's export data and fails on every file with
an unhelpful message about import versions. The pinned version moves when the
language version does.

## Licences

Shipped dependencies must be permissively licensed: Apache-2.0, BSD-2-Clause,
BSD-3-Clause, ISC, MIT or MPL-2.0 — SCP-06. Checked two ways, because they catch
different things: `make licences` walks what the module actually links, and
dependency review inspects what a pull request adds.

Build tooling is exempt (SCP-07). The linter is GPL-licensed; running a tool
over the code affects its licence no more than the compiler does.

## The API document

Generated from the operations registered in `internal/httpapi`, never written by
hand — API-04. CI regenerates it and fails if the committed copy differs, so an
endpoint cannot change without the document following.

The application does not serve it (API-05). Documentation is published
separately, which leaves the binary with no unauthenticated routes except the
probes below.

## Probes

`/healthz` and `/readyz` answer without authentication and report nothing beyond
whether the process is up.

This is the one exception to every request being authenticated (ACC-03), and it
is unavoidable: a container probe has no way to sign in. They are deliberately
outside the documented API, and must never grow a response body that says
anything about the system's contents.

## Documentation

Built with mkdocs-material and published to GitHub Pages on every push to `main`
— API-10. Sets are versioned with `mike` so a reader can tell which release they
are reading — API-11. Unreleased work publishes as `main`; a tagged release
publishes under its version and moves the `latest` alias.

## Repository settings the gate depends on

Two checks rely on settings that live in the repository rather than in a
workflow file, and both fail unhelpfully when the setting is off:

| Setting | Needed by | Symptom when off |
|---|---|---|
| Dependency graph, via Dependabot alerts | Dependency review | "Dependency review is not supported on this repository" |
| Pages, serving the `gh-pages` branch | Documentation publishing | The workflow succeeds and nothing is served |

Secret scanning and push protection are on by default for public repositories.

## Branch protection

Not enforced yet — CIG-08. The gate runs on every pull request but nothing
blocks a merge, which is exactly the state CIG-01 warns about. This is a
deliberate choice for early development and needs revisiting before the project
takes outside contributions.
