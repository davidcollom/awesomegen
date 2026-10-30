# syntax=docker/dockerfile:1
#
# Standalone, multi-stage build of awesomegen.
#
# The build stage compiles a statically linked binary, the runtime stage
# contains nothing but that binary and the CA certificates and runs as a
# non-root user.
#
# The published release image is built by GoReleaser instead (see
# Dockerfile.goreleaser), so that the image contains exactly the binary that
# is published and signed. This Dockerfile is used for local builds, for
# `docker buildx build` and for the container smoke test in CI.

# ---- build stage ------------------------------------------------------------
# Pinned by digest: a tag on its own is mutable and could be re-pointed at a
# different base image at any time.
#
# To refresh the pin:
#   docker buildx imagetools inspect golang:1.26-alpine --format '{{ .Manifest.Digest }}'
FROM golang:1.26-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS builder

WORKDIR /src

# git is only needed when the Go toolchain has to fall back to fetching a
# module directly instead of through the module proxy.
RUN apk add --no-cache git ca-certificates

# Dependencies first, so that this layer stays cached while the sources
# change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TARGETOS/TARGETARCH are provided by buildx. The defaults keep a plain
# `docker build` working.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

# CGO is disabled, so the binary is fully static and runs on a base image
# that has no C library at all.
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/awesomegen ./cmd/awesomegen

# ---- runtime stage ----------------------------------------------------------
# Pinned by digest, see the note above on how to refresh it.
FROM gcr.io/distroless/static:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

ARG VERSION=dev

LABEL org.opencontainers.image.title="awesomegen" \
      org.opencontainers.image.description="Generate Awesome lists from GitHub Star Lists" \
      org.opencontainers.image.source="https://github.com/davidcollom/awesomegen" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=builder /out/awesomegen /usr/local/bin/awesomegen

# The distroless "nonroot" user, uid/gid 65532.
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/awesomegen"]
