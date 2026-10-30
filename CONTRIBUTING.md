# Contributing to awesomegen

Thanks for taking the time to contribute!

`awesomegen` is a small Go CLI and GitHub Action that turns GitHub Star
Lists into Awesome-list `README.md` files. It is maintained by
[@davidcollom](https://github.com/davidcollom).

---

## Prerequisites

| Tool       | Version                                               |
| ---------- | ----------------------------------------------------- |
| Go         | **1.26 or newer** (see the `go` directive in `go.mod`) |
| Git        | any recent version                                     |
| Docker     | optional, only needed for the container image          |
| `make`     | any recent version                                     |

Nothing else has to be installed: the linter, GoReleaser and govulncheck
are executed through `go run` with a pinned version, so `make lint` and
friends work straight away.

```bash
git clone https://github.com/davidcollom/awesomegen.git
cd awesomegen
make build
./bin/awesomegen --version
```

---

## Make targets

| Target              | What it does                                                  |
| ------------------- | ------------------------------------------------------------- |
| `make help`         | list every target                                              |
| `make build`        | build the CLI into `bin/`                                      |
| `make install`      | install the CLI into the Go bin directory                      |
| `make test`         | run the unit tests                                             |
| `make test-race`    | run the unit tests with the race detector (needs cgo)         |
| `make cover`        | run the tests and write `coverage.out`                         |
| `make lint`         | run golangci-lint                                              |
| `make fmt`          | format the source with `gofmt`                                 |
| `make fmt-check`    | fail when the source is not formatted                          |
| `make vet`          | run `go vet`                                                   |
| `make schema`       | regenerate `config-schema.json`                                |
| `make schema-check` | fail when `config-schema.json` is out of date                  |
| `make tidy`         | run `go mod tidy`                                              |
| `make vuln`         | run govulncheck against the dependency graph                   |
| `make release`      | cut a release with GoReleaser                                  |
| `make snapshot`     | build a snapshot release with GoReleaser (nothing is published) |
| `make clean`        | remove `bin/`, `dist/` and `coverage.out`                      |

`make release` and `make snapshot` need a clean working tree, because
GoReleaser refuses to publish from a dirty tree.

---

## Running the tests

```bash
make test          # unit tests
make test-race     # with the race detector (CGO_ENABLED=1)
make cover         # coverage profile in coverage.out
```

The tests do not touch the network: the scraper runs against the HTML
fixture in `testdata/`, which is served by a local test server, the clock is
controlled with `clockwork`, and the GitHub client is exercised through the
fake it takes as an interface.

---

## Regenerating the JSON schema

`config-schema.json` is generated from the Go types in
`internal/config/types.go`. The `//go:generate` directive in that file runs
`internal/config/generate_schema.go`, so the whole thing is one command:

```bash
make schema
```

CI fails when the committed schema does not match the generated one, so
always regenerate and commit the file together with the type change:

```bash
make schema
git add config-schema.json
```

The generator deliberately writes no timestamp, so the output is
byte-for-byte reproducible and the drift check does not produce false
positives.

---

## Code style

* `gofmt` is mandatory; `make fmt-check` and the CI `lint` job enforce it.
* `golangci-lint` runs with the configuration in `.golangci.yml`. The enabled
  linters are `errcheck`, `govet`, `staticcheck`, `unused`, `ineffassign`,
  `misspell`, `gocritic` and `revive`.
* Prefer small, focused functions with a doc comment that explains *why*,
  not *what*.
* Errors are wrapped with `%w` and given context, for example
  `fmt.Errorf("parse config %s: %w", path, err)`.

---

## Commit messages

The project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <short summary>
```

Types that are used: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`,
`build`, `ci`, `chore`, `security`.

```
feat: support multiple sources per list
fix(render): do not emit an empty table of contents
docs: document the topic grouping modes
chore(deps): bump the go-dependencies group
```

The changelog that GoReleaser generates for a release filters out `docs:`,
`test:` and `chore(deps):` commits and groups the rest into *Features*,
*Bug fixes*, *Documentation* and *Others*. Use the imperative mood and keep
the summary under ~72 characters.

---

## Pull requests

Before you open a pull request:

1. `make fmt-check vet lint test` passes.
2. `make schema` produces no diff (or the regenerated `config-schema.json`
   is part of the pull request).
3. The change is covered by a test. New behaviour needs a test, bug fixes
   need a regression test.
4. The pull request describes *what* changed and *why*; the commit messages
   describe the individual steps.
5. Keep the pull request focused. Unrelated reformatting makes the review
   harder and slows it down.

CI runs on every push to `main` and on every pull request: `gofmt`, `go vet`,
`golangci-lint`, the unit tests on Linux, macOS and Windows with Go 1.26 and
1.27, a cross compilation for every release target, a container build and
`govulncheck`.

---

## How a release is cut

1. Make sure `main` is green and that the version you want to publish does
   not exist yet.
2. Merge the release pull request (usually a `chore(release): vX.Y.Z`
   commit that only updates the version references).
3. Tag the merge commit and push the tag:

   ```bash
   git tag -a v1.2.3 -m "v1.2.3"
   git push origin v1.2.3
   ```

   The tag must match `v*.*.*`; the release workflow only listens for that
   pattern.

4. The `release` workflow then:
   * builds the binaries for `linux`, `darwin` and `windows` on `amd64` and
     `arm64` with GoReleaser, with reproducible timestamps;
   * creates the archives, the SHA-256 checksum file and a per-archive SPDX
     SBOM;
   * publishes the GitHub release and pushes the multi-architecture
     container image to `ghcr.io/davidcollom/awesomegen`;
   * signs the archives and the checksum file with cosign, keyless, using
     the workflow's OIDC identity;
   * attaches an aggregate SBOM and the signatures to the release;
   * records a GitHub build provenance attestation for every archive.

5. Verify the release:

   ```bash
   gh attestation verify awesomegen_1.2.3_linux_amd64.tar.gz --repo davidcollom/awesomegen
   cosign verify-blob \
     --certificate-identity 'https://github.com/davidcollom/awesomegen/.github/workflows/release.yml@refs/tags/v1.2.3' \
     --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
     --bundle checksums.txt.sigstore.json checksums.txt
   ```

To rehearse a release without publishing anything, use `make snapshot`. The
snapshot version carries a `-next` suffix so it can never be confused with a
real release. The snapshot skips the SBOM, the signatures and the container
image: they need syft, cosign with an OIDC token and a Docker daemon, and
none of the three is published anyway.

---

## Reporting security problems

Please do not open a public issue for a security problem. Follow the
disclosure process in [SECURITY.md](SECURITY.md).

---

## Code of conduct

Be respectful. Assume good faith. Reviews are about the code, not about the
person who wrote it.
