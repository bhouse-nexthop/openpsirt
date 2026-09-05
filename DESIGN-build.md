# Build and validation

How a change gets from an editor to `main`, and what stops it if it should not.

Satisfies CIG-01 to CIG-08, SCP-06, SCP-07, SEC-10, API-04, API-10, API-11,
API-13, API-15.

## Layout

| Path | Holds |
|---|---|
| `cmd/openpsirt/` | The binary. Flag parsing, configuration, server lifecycle |
| `internal/version/` | What this build is. Values injected at link time |
| `internal/config/` | Settings, read from the environment with working defaults |
| `internal/httpapi/` | The HTTP surface and the operations the API document is generated from |
| `internal/database/` | Opening, identifying and validating a database. See `DESIGN-database.md` |
| `internal/database/migrate/` | The migration runner and the locks around it |
| `internal/database/migrate/migrations/` | The migrations themselves |
| `internal/schema/` | Applies this application's schema. Exists so that using the runner registers the migrations |
| `internal/dbtest/` | Runs a test against every database available to it |
| `internal/catalog/` | Products, streams and variants. See `DESIGN-data-model.md` |
| `internal/ingest/` | What happens to an arriving scan. See `DESIGN-ingest.md` |
| `internal/queue/` | Durable background work. See `DESIGN-queue.md` |
| `internal/sbom/`, `internal/scanner/` | Reading an inventory, and running the scan over it. See `DESIGN-ingest.md` |
| `internal/graph/`, `internal/finding/` | The dependency graph, and what a scan found. See `DESIGN-data-model.md`, `DESIGN-findings.md` |
| `internal/triage/`, `internal/advisory/` | Judgments and their approvals, and the CSAF document generated from them. See `DESIGN-triage.md` |
| `internal/access/`, `internal/signin/` | Who is asking, and how they arrived. See `DESIGN-access.md` |
| `internal/notify/` | What people are told about. See `DESIGN-notifications.md` |
| `internal/markdown/`, `internal/setting/`, `internal/currency/` | What may be written, what an administrator may change, and asking package indexes what is current |
| `internal/webui/` | The built interface, embedded. See `DESIGN-interface.md` |
| `internal/docs/`, `internal/tools/` | The checks that read these documents, and the gates that are not linters |
| `web/` | The interface's source. See `DESIGN-interface.md` |
| `deploy/helm/openpsirt/` | The chart. See `DESIGN-packaging.md` |
| `docs/` | The published documentation site |
| `assets/` | Logo files |

Everything is under `internal/`, so nothing is importable by another module
until we deliberately decide otherwise.

## One entry point

Every check CI runs is a `make` target. A developer reproduces any CI failure
with the same command and the same pinned tool versions — CIG-05.

| Target | Does |
|---|---|
| `make build` | Builds the binary with version information injected |
| `make test` | The quick loop: SQLite only, packages in parallel, cached. Seconds, so it runs after every change |
| `make test-all` | Every configured engine, race detector on, nothing cached. What `check` runs |
| `make lint` | Static analysis, pinned version |
| `make govulncheck` | Known vulnerabilities in dependencies |
| `make licenses` | Licenses of shipped dependencies against the allowlist |
| `make openapi` | Regenerates the API document from the code |
| `make sbom` | Generates our own CycloneDX bill of materials |
| `make web-check` | The interface: its dependencies installed as locked, the type check, its tests, and the generated client diffed against the API document |
| `make vet` | The compiler's own checks |
| `make unreachable` | Exported code nothing reaches, which a linter reporting only unexported symbols will not find |
| `make unclaimed` | Every decision in force is named by a design document |
| `make openapi-current` | The committed API document and privileges page match what the code generates |
| `make check` | Everything above. CI runs the last four as their own steps as well |
| `make check-engines` | That all four engines ran, and that each was the engine it claimed |
| `make check-packaging` | The container image and the Helm chart. Needs docker and helm |
| `make engines-up` | Starts the four database servers the suite needs, and records their URLs |
| `make engines-down` | Removes them |
| `make engines-status` | What is running, and which engines are unconfigured |
| `make measure` | Measurements rather than gates. Behind a build tag, so `check` never runs them |

## The engines a developer tests against

The suite runs against SQLite alone unless it is pointed at real servers, and
**a skipped engine passes** — so a green run does not mean four engines agreed,
it means nothing failed, which is also what running almost nothing looks like.
Testing against all four is therefore ordinary development rather than
something CI does afterwards (DAT-12), and standing the servers up is a target
rather than a paragraph each machine follows by hand.

`make engines-up` starts them, waits until each is answering, and writes their
URLs to `local.mk`, which is git-ignored and included by the makefile if
present. So a machine is configured once rather than once per command, and a
checkout that has one is not different from a checkout that does not.

Four properties of it are deliberate:

**A container reported "Up" is not one that answers.** Each server is asked
with its own client, inside its own container, so nothing depends on a client
being installed on the machine — and a server that never answers fails the
target loudly rather than leaving the suite to fail confusingly later.

**`local.mk` is never overwritten.** It is machine-local, and somebody may be
pointing at servers of their own; replacing that silently is how a run tests
something other than what its author believes it is testing.

**Starting is idempotent.** An engine already running is left alone and a
stopped container is started again rather than replaced, so what is in it
survives.

**The images are pinned, and checked against CI's.** A local four-engine pass
means what CI's pass means only if they are the same servers, and two pinned
lists in two files drift — invisibly, and exactly when it matters. The check
runs in both directions, so an engine CI adds that nothing here starts is
caught as well as the reverse, and **CI runs it too**: a check that only fires
on the machine that already agrees is one nobody sees fail.

One of the four is deliberately **below the supported version floor**, so the
refusal to run against an old server is exercised rather than skipped.

The URLs are read when make starts, so the run that writes `local.mk` is not
the run that uses it.

## The review checklist is worked, not remembered

The checklist in `AGENTS.md` is worked through on every review rather than
consulted when somebody remembers (SEC-10). That is the difference between a
control and a document: a list nobody opens is a list that agrees with whatever
was done.

It is not enforced by the pipeline, and saying so is the point. What CI can
check, CI checks — the gate below is long precisely so that the checklist is
left holding only what a machine cannot decide. What remains there is judgment,
and judgment that is skipped is invisible unless working the list is a step
somebody takes deliberately.

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

## Licenses

Shipped dependencies must be permissively licensed: Apache-2.0, BSD-2-Clause,
BSD-3-Clause, ISC, MIT or MPL-2.0 — SCP-06. Checked two ways, because they catch
different things: `make licenses` walks what the module actually links, and
dependency review inspects what a pull request adds.

A short exception list covers modules whose license the classifier cannot read.
Each entry names the license and why the tool fails on it, and the license has
been read by hand before being added. The alternative — lowering the classifier's
confidence threshold — would accept every other unreadable license silently,
which is the opposite of what the check is for.

Build tooling is exempt (SCP-07). The linter is GPL-licensed; running a tool
over the code affects its license no more than the compiler does.

## The API document

Generated from the operations registered in `internal/httpapi`, never written by
hand — API-04. CI regenerates it and fails if the committed copy differs, so an
endpoint cannot change without the document following. The privileges page
(API-22) comes off the same registrations and is checked the same way, in the
same job: a generated file nothing diffs is a hand-maintained file with extra
steps.

The application serves the document itself — the framework's own route,
authenticated like every other — and nothing that renders it: no documentation
page (API-05). Documentation is published separately, which leaves the binary
with no unauthenticated routes except the probes below.

## Our own bill of materials

We ingest SBOMs, so we publish one — CycloneDX, the format this project treats
as authoritative on the way in. Generated from the built binary's module graph
rather than from source, so it describes what ships (SCP-08, SCP-09).

License fields are not populated reliably by the generator, and nothing depends
on them: license compliance is gated by the allowlist check instead (SCP-10).

Once ingest exists, this file is the project's own first test fixture.

## Probes

`/healthz` and `/readyz` answer without authentication and report nothing beyond
whether the process is up.

Unavoidable: a container probe has no way to sign in. They are deliberately
outside the documented API, and must never grow a response body that says
anything about the system's contents.

They are not the only routes that answer without a credential — the sign-in
paths and the interface's own assets do too, for reasons `DESIGN-api.md` sets
out. What is true of all of them is that none reads anything from the database
about what this deployment holds (ACC-03).

## Documentation

Built with mkdocs-material and published to GitHub Pages on every push to
`main` — API-10. Sets are versioned with `mike` so a reader can tell which
release they are reading — API-11. **Today the workflow publishes one set,
`main`, and is the default**; publishing a tag under its version and moving a
`latest` alias belongs with a release process, which does not exist yet. The
versioning machinery is in place so that the first release is a workflow
trigger rather than a rebuild.

The configuration page lists every environment variable the process reads,
with its meaning and default. A variable that is set and cannot be read stops
the process with the variable named, rather than falling back: a switch spelled
wrongly that silently reads as its opposite is worse than a refusal to start.

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
