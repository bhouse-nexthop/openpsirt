# Packaging and deployment

How openpsirt is delivered, and what a deployment looks like.

Satisfies SCP-03, SCP-12 to SCP-14, and the probe behaviour DAT-10 requires.

## The image

Multi-stage: built with the Go toolchain the module asks for, run on Alpine.

The binary is **fully static** — CGO is off, which is part of why the pure-Go
SQLite driver was chosen. That means the runtime image could be `scratch` or
distroless and carry no shell at all.

Alpine is used anyway, deliberately. A self-hosted operator debugging their own
deployment wants a shell, and that is worth the few megabytes and the extra
surface. It is one line to change if that trade ever stops being worth it.

| | |
|---|---|
| Size | ~40 MB |
| User | Non-root, no login shell, no home directory |
| Extras | Root certificates, for outbound TLS to the ranking feeds. Timezone data |
| Healthcheck | Liveness only — readiness needs the database and belongs to the orchestrator, which can act on it |

CI builds the image, runs it, and **checks it is not running as root**. That is
the only place the claim is tested rather than asserted.

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
seccomp profile. Nothing here needs any of them. `/tmp` is an `emptyDir`,
because a read-only root still needs somewhere to put a scratch file.

The service account has `automountServiceAccountToken: false` — openpsirt never
talks to the Kubernetes API, so it has no use for a token that would otherwise
sit in every pod.

### The database URL

From a Secret, always. Either name one you manage, or let the chart create one
from a value — with the caveat stated in the values file that a password put
there ends up in your release history and probably in version control.

Setting both, or neither, **fails at render time with a sentence naming the
problem**. Rendering manifests that cannot work would move the failure to a
crash-looping pod and a message nobody reads.

## What is not here yet

Publishing. The image builds and is verified in CI but is not pushed anywhere,
and the chart is not published to a repository. Both belong with a release
process, which does not exist yet.
