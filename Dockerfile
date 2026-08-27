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
RUN addgroup -S openpsirt && adduser -S -G openpsirt -H -s /sbin/nologin openpsirt

COPY --from=build /out/openpsirt /usr/local/bin/openpsirt

USER openpsirt
EXPOSE 8080

# Liveness only. Readiness depends on the database and belongs to the
# orchestrator, which can act on it; a container healthcheck cannot.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/openpsirt", "-version"]

ENTRYPOINT ["/usr/local/bin/openpsirt"]
