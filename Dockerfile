# Build. Pinned to the same Go the module asks for.
FROM golang:1.27-alpine AS build

WORKDIR /src

# Dependencies first, so a source-only change does not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# CGO off gives a static binary, which is what lets the runtime image be this
# small — and is why the pure-Go SQLite driver was chosen.
ENV CGO_ENABLED=0
RUN go build -trimpath \
      -ldflags "-s -w \
        -X 'github.com/bhouse-nexthop/openpsirt/internal/version.version=${VERSION}' \
        -X 'github.com/bhouse-nexthop/openpsirt/internal/version.commit=${COMMIT}' \
        -X 'github.com/bhouse-nexthop/openpsirt/internal/version.date=${DATE}'" \
      -o /out/openpsirt ./cmd/openpsirt

# Run.
#
# The binary is static, so this could be scratch or distroless and carry no
# shell at all. Alpine is chosen deliberately: a self-hosted operator debugging
# their own deployment wants a shell, and that is worth the few megabytes and
# the surface it adds.
FROM alpine:3.21

# Outbound TLS needs root certificates: the ranking feeds are fetched over
# HTTPS, and without these every fetch fails with an unhelpful error.
RUN apk add --no-cache ca-certificates tzdata

# Runs as a normal user. Nothing here needs root, and a container that does not
# need it should not have it.
# The identifiers are pinned. Letting adduser choose meant the chart's
# runAsUser matched only by luck, and any future package that creates a system
# user would shift it — after which the pod runs as a different user than the
# chart declares, silently.
RUN addgroup -g 65532 -S openpsirt \
 && adduser -u 65532 -S -G openpsirt -H -s /sbin/nologin openpsirt

COPY --from=build /out/openpsirt /usr/local/bin/openpsirt

USER openpsirt
EXPOSE 8080

# Liveness. Readiness depends on the database and belongs to the orchestrator,
# which can act on it.
#
# This asks the running server, not a new process. Running "openpsirt -version"
# proved only that the binary was on disk and executable — a deadlocked or
# non-listening server passed it forever.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/openpsirt"]
