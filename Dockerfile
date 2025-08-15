# Dockerfile.binary
# This image expects the compiled binary to be present in the build context
# as `awesomegen` (GoReleaser takes care of that).

FROM gcr.io/distroless/base:nonroot

LABEL org.opencontainers.image.title="awesomegen" \
      org.opencontainers.image.description="Generate Awesome lists from GitHub Star Lists" \
      org.opencontainers.image.source="https://github.com/davidcollom/awesomegen" \
      org.opencontainers.image.licenses="MIT"

COPY awesomegen /usr/local/bin/awesomegen

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/awesomegen"]