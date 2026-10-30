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
# The version of Go is the one that go.mod requires. A Dockerfile cannot read
# the `go` directive of go.mod - the base image is named before anything is
# copied into the build - so the version is written here as well. The digest
# pins the image, so the two versions only drift apart when the directive of
# go.mod is changed, and the build fails with a clear message when that
# happens.
#
# To refresh the pin:
#   docker buildx imagetools inspect golang:1.27.3-alpine --format '{{ .Manifest.Digest }}'
FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS builder

WORKDIR /src

# git is only needed when the Go toolchain has to fall back to fetching a
# module directly instead of through the module proxy.
RUN apk add --no-cache git ca-certificates

# Dependencies first, so that this layer stays cached while the sources
# change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TARGETOS and TARGETARCH are provided by BuildKit. They are declared
# without a default on purpose: a default would override the automatic
# value, and a cross-platform build would then compile for the wrong
# architecture and copy that binary into an image of another one.
#
# The shell defaults below only apply to a build with a builder that does
# not supply them at all, such as the classic builder.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# CGO is disabled, so the binary is fully static and runs on a base image
# that has no C library at all.
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}" \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
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
