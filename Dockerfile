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
# The vulnerability scanner. This deployment runs the scan rather than trusting
# whatever a producer's pipeline happened to install, so an image without one
# takes in inventories it can never answer a question about.
#
# Pinned and checksummed. Which scanner build answered is part of what a
# finding means — counts are only comparable between products measured the same
# way — so "whatever was latest at build time" is not good enough.
FROM alpine:3.21 AS scanner
ARG GRYPE_VERSION=0.112.0
ARG TARGETARCH=amd64
RUN apk add --no-cache curl ca-certificates \
 && case "${TARGETARCH}" in \
      amd64) expected=acb14a030010fe9bdb9594b4ae108d9d14ef2f926d936aa0916dc62c89c058ea ;; \
      arm64) expected=7fdeccf065965cc59386c656e5fcc1eb1bdf820e2433000bca7f010b8e6da155 ;; \
      *) echo "no pinned checksum for ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
 && curl -fsSL -o /tmp/grype.tar.gz \
      "https://github.com/anchore/grype/releases/download/v${GRYPE_VERSION}/grype_${GRYPE_VERSION}_linux_${TARGETARCH}.tar.gz" \
 && echo "${expected}  /tmp/grype.tar.gz" | sha256sum -c - \
 && mkdir -p /out \
 && tar -xzf /tmp/grype.tar.gz -C /out grype \
 && chmod 0755 /out/grype

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
COPY --from=scanner /out/grype /usr/local/bin/grype

# Where the scanner keeps its vulnerability data, and where this process looks
# for the scanner. Both are set so that a deployment which configures nothing
# still scans.
#
# The data is fetched at runtime rather than built in. A database baked into an
# image is stale the day after it is published, and the reason scanning happens
# here at all is that the data moves under an inventory that does not. The
# directory has to be writable, which with a read-only root filesystem means a
# mounted volume — the chart provides one.
ENV GRYPE_DB_CACHE_DIR=/var/cache/openpsirt/grype \
    OPENPSIRT_SCANNER_PATH=/usr/local/bin/grype
RUN mkdir -p /var/cache/openpsirt/grype \
 && chown -R 65532:65532 /var/cache/openpsirt

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
