# Agent instructions for OpenPSIRT

OpenPSIRT ingests SBOMs that already carry vulnerability data, tracks what
changes release to release, and gives people somewhere to triage findings and
follow them through to a fix.

**Read `DECISIONS.md` before proposing anything.** Most questions have already
been answered there, with the reasoning. If you think a decision is wrong, say
so and cite its ID — do not quietly implement something else.

## Document map

| File | Holds | Lifetime |
|---|---|---|
| `DECISIONS.md` | **Why** — every decision, with reasoning, by area | Permanent |
| `DESIGN-*.md` | **How** — structures, flows, behaviour | Permanent |
| `IMPLEMENTATION.md` | Build order | **Temporary — deleted when the work lands** |
| Code | What runs | — |

### The chain that makes audits possible

Code satisfies a design document. A design document names the decision IDs it
implements. A decision says why.

**If code does something no design document describes, it is a remnant.** Either
the design document is out of date or the code should not be there. Both get
re-examined; neither is assumed correct. This is the whole reason the design
documents exist, so keeping them current is part of the change, not follow-up
work.

## Design documents

All at the repository root, named `DESIGN-<area>.md`.

- **Keep them language-agnostic.** They describe behaviour, architecture and
  domain concepts — SBOM structure, dependency paths, triage outcomes,
  visibility rules. Never type names, struct fields, function signatures or
  source paths. Implementation pointers belong in code.
- **Name the decisions they satisfy**, by ID. That is the audit trail.
- **Record what the decisions did not cover.** Plenty gets chosen while writing
  code that no decision anticipated. Those choices are exactly what an auditor
  cannot distinguish from an accident, so write them down.
- **Update them in the same change as the code.** A design document that lags is
  worse than none, because it is trusted and wrong.

## Plan documents are temporary

`IMPLEMENTATION.md` will be deleted once its work has landed.

**Never reference a plan document or its stage numbers** from code, comments,
commit messages, test names or design documents. "Stage 3", "as planned in
Stage 1" and similar become dead pointers the moment the file goes. Refer to the
*behaviour* instead, and link to a `DESIGN-*.md` section.

Name regression tests for the invariant they pin, not for where the work sat in
a plan.

## Non-negotiables

These are the decisions easiest to violate by accident, and most expensive to
unpick later.

| Rule | Decision |
|---|---|
| **No engine-specific SQL in the core.** Four engines are supported. Engine-specific code is confined to migration DDL and the job queue's locking, and nowhere else | DAT-02 |
| **Never trust identifiers a scan file supplies** to be stable between builds or consistent between producers. Identity is derived from content | ING-05, MDL-06 |
| **Visibility is enforced in the data-access layer**, never per handler, and every query carries a subject. This covers counts, aggregates, search and exports — not just row reads | ACC-04, ACC-07 |
| **A finding is a component at a specific place.** Do not deduplicate up to the package. Grouping is presentation only | MDL-05, REL-02 |
| **Identity is structural; expiry is version-based.** Never mix them — that is how an unrelated top-level bump invalidates a leaf decision | MDL-08 |
| **The tests run on every supported engine.** SQLite-only tests catch none of the portability traps | DAT-12 |
| **SQL values are parameterized and SQL identifiers come from an allowlist.** A placeholder cannot bind a column name, so a sort column from a query parameter is the real risk | SEC-01, SEC-02 |

## Source file size

**Target about 1200 lines per source file.** A point to split at, not a hard
limit.

A file past it is usually doing more than one thing. The cost is not storage —
it is that nobody reads to the end, changes get made in the wrong place, and
review gets shallower the further down the diff it goes.

Split by **responsibility, not by line count**. Cutting a coherent file in half
to hit a number makes it worse, and a 1300-line file that genuinely does one
thing is better left alone than split badly. Act on the trend rather than the
threshold: splitting late is far more work than splitting early.

Documentation is not governed by this. A design document is as long as its
subject, and splits when it covers two subjects rather than when it reaches a
length.

## Decision identifiers

**Keep rows sorted by identifier within each table.** Adding a decision next to
a related one is the natural instinct and it scrambles the order — which makes a
reference document hard to scan and makes it look like entries are missing.
Append, sort, and use a cross-reference to point at the related decision.

Identifiers never change. Renumbering breaks every commit message and design
document that cites one.

## Code conventions

**No implementation-timeline language in code or comments.** Never write "for
now", "temporarily", "first cut", "later", "in this phase", "step N", or
anything describing *when* in the build something happens. Such notes rot the
moment the next change lands. Comment the current behaviour and why. If
something genuinely is missing, describe the missing behaviour or the
limitation — not when it will arrive.

**No ticket or tracker references in code, comments or documents.** A bare
number is unactionable at the code and rots as work is split or superseded.
Describe the behaviour, reason or limitation instead, and keep issue linkage in
the tracker and the pull request.

**Comment density matches the surrounding code.** Explain why, not what.

**The name is written OpenPSIRT.** In prose and in anything a person reads —
documentation, the API description, the version the binary prints, a chart
description. It is a product name, and a product name that appears in three
casings reads like three different things.

Lower case is for what someone types or a machine matches: the command
(`openpsirt migrate up`), the module path, the container image, the chart, the
`OPENPSIRT_` environment prefix, and paths like `cmd/openpsirt/`. Those are
identifiers rather than the name, and changing them would break what people
already have.

## Security review checklist

Worked through on every review. Each line says what to look for **in this
codebase**, not in general — a checklist that restates the category is one that
gets ticked without being read.

| Category | Check |
|---|---|
| **Injection — SQL** | Every value parameterized. **Every identifier from an allowlist** — sort columns, filter fields and partition names cannot be bound by a placeholder, so a column name arriving from a query parameter is the live hole (SEC-01, SEC-02) |
| **Injection — output** | Component names, versions and descriptions come from a third party's SBOM and get rendered to staff who hold the most access. Encoded on output, and **never passed through the markdown renderer** — that is for text a person typed here (SEC-04, SEC-16) |
| **Injection — markdown** | Policy enforced at submission, **before storage**: raw HTML off at the parser rather than stripped after, link schemes limited to `http`, `https`, `mailto`, **nothing remote fetched by a rendered document, images included**. Source stored, never rendered HTML, and the sanitiser still runs on render because stored text predates later rules. **The fenced-block language tag is input** — allowlisted before it reaches a class attribute (SEC-11 to SEC-15, SEC-18) |
| **Broken access control** | Enforced in the data layer with a subject, never per handler. Covers counts, aggregates, search and exports — not just row reads (ACC-04, ACC-07). An attachment fetch is authorised against its finding's visibility before any signed URL is issued (ATT-06) |
| **Cryptographic failures** | API keys and personal tokens hashed at rest, shown once (SEC-03). Session identifiers unguessable |
| **Insecure design** | Does the change contradict a recorded decision? Cite the identifier if so |
| **Security misconfiguration** | Container non-root with a read-only root filesystem. Trusted-header sign-in off by default and only from configured sources (ACC-19, ACC-20) |
| **Vulnerable components** | `govulncheck` and dependency review gate. A new dependency needs a permissive licence (SCP-06) |
| **Authentication failures** | No account created automatically on any path; unauthorized users get a generic message that does not say why (ACC-21) |
| **Data integrity** | A scan file is hostile input. Bounded size, depth and component count; never used as a filesystem path (SEC-05, SEC-06). Markdown fields are length-bounded and rendering is time-bounded (SEC-17) |
| **Separation of duties** | An approval points at one revision of a justification. Anything that lets approved text change without withdrawing the approval defeats TRI-07 silently (TRI-24, TRI-25) |
| **Logging failures** | Secrets never logged. Triage actions land in the append-only history (SEC-08) |
| **Request forgery** | Outbound fetches restricted to their configured host, no redirects into private address space (SEC-07) |

## Testing

- Every change runs against all four database engines in CI.
- Test fixtures include a real SBOM per supported producer, plus one full-size
  fixture for performance work.
- **Authorization is tested as a matrix**: role × visibility × endpoint,
  including counts, aggregates, search and exports.
- Regression tests are named for the invariant they pin.

## Commits and pull requests

- **No `Co-Authored-By` trailers.** This project will use DCO, where the only
  trailer that carries meaning is `Signed-off-by`. A co-author trailer asserts
  authorship that nobody has signed for, and mixing the two makes the sign-off
  chain ambiguous. Tools that add one by default must be told not to.
- Explain **why** in the body. The diff already shows what.
- Design document updates belong in the same commit as the behaviour they
  describe.
- Branch protection is not enforced yet — early development. The static analysis
  gate still runs; do not push past it on the grounds that nothing blocks you.

## Building

Everything CI runs is a `make` target, so a failure reproduces locally with the
same command and the same pinned versions.

| | |
|---|---|
| `make check` | Everything CI checks, except the container and chart |
| `make check-packaging` | The container image and the Helm chart. Needs docker and helm |
| `make build` | The binary, with version information injected |
| `make openapi` | Regenerates the API document from the code |

Tests run against SQLite alone by default. To exercise the production engines,
point these at real servers — CI does, and a change that only ran on SQLite has
tested none of the portability traps:

    OPENPSIRT_TEST_POSTGRES_URL   OPENPSIRT_TEST_MYSQL_URL
    OPENPSIRT_TEST_MARIADB_URL    OPENPSIRT_TEST_TOO_OLD_URL
