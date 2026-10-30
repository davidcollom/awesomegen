# awesomegen backlog

The work that is left, and where it is tracked.

Every item that is ready to be picked up has a GitHub issue of its own, and
every issue names the files it touches, what the change should do and the
test that has to fail without it. The items that are still written down here
are the ones that need a decision or a design first.

**P0** - do before the next release; they affect correctness, security or the
ability to ship.
**P1** - do soon; they remove friction or close a real gap.
**P2** - nice to have; they improve the project but nothing is blocked.

Status: `open` unless stated otherwise.

---

## P0 - correctness and shipping

| Issue | What is missing |
|---|---|
| [#106](https://github.com/davidcollom/awesomegen/issues/106) | The HTTP path of the GitHub client is not covered by a test - `New`, `GetRepo` and `IsNotFound` are at 0 % |
| [#107](https://github.com/davidcollom/awesomegen/issues/107) | There is no end-to-end test of the command line interface - nothing runs the scrape, the enrichment, the rendering and the write together |
| [#108](https://github.com/davidcollom/awesomegen/issues/108) | Nothing fails when the coverage drops - the profile is uploaded and the summary is printed, but no minimum is enforced |
| [#109](https://github.com/davidcollom/awesomegen/issues/109) | The Star List page shape is only checked by a fixture, so a change of the markup of GitHub is noticed by a user and not by the build |
| [#116](https://github.com/davidcollom/awesomegen/issues/116) | The exit code of the command keeps the `exitAfterDefer` check of `gocritic` disabled |

## P1 - gaps that should be closed soon

| Issue | What is missing |
|---|---|
| [#111](https://github.com/davidcollom/awesomegen/issues/111) | Dependabot does not watch `Dockerfile.goreleaser`, so its digest pin goes stale |
| [#112](https://github.com/davidcollom/awesomegen/issues/112) | There is no `.gitattributes`, so a Windows checkout rewrites the line endings of the repository |
| [#113](https://github.com/davidcollom/awesomegen/issues/113) | The `CODEOWNERS` file routes every change to one person |
| [#114](https://github.com/davidcollom/awesomegen/issues/114) | The action resolves the latest release by default, so a workflow file is not reproducible over time |
| [#115](https://github.com/davidcollom/awesomegen/issues/115) | There is no `CHANGELOG.md`, so the history of the project only lives on the release page |
| [#86](https://github.com/davidcollom/awesomegen/issues/86) | The documented `docker run` command cannot write the README |

## P2 - lower priority, but actionable

| Issue | What is missing |
|---|---|
| [#110](https://github.com/davidcollom/awesomegen/issues/110) | There is no scheduled run of the pipeline, so a change of Go or of the GitHub API is noticed by a user and not by the build |

## P2 - ideas that need a decision first

### macOS binaries are not notarized
The `notarize` block is present in `.goreleaser.yaml` but commented out,
because it needs a paid Apple Developer account, a "Developer ID
Application" certificate and an App Store Connect API key.
* Decide whether to buy the membership and enable the block, or to state
  explicitly in the README that the macOS binaries are unsigned.

### Homebrew, Scoop and Nix packaging
The release already publishes binaries for macOS and Windows.
* Add the `homebrew`, `scoop` and `nix` sections to `.goreleaser.yaml`; they
  are supported by GoReleaser v2 out of the box.

### Use the GitHub API instead of scraping HTML
`internal/scrape` parses HTML because the Star List API is not public.
* Investigate whether the GraphQL API exposes star lists after all; that
  would remove the whole scraping code path and its fragility.

### Cache the GitHub API responses
Every run looks up every repository again.
* Add an on-disk cache keyed by repository and by ETag, with a short TTL.

### Streaming rendering for very large lists
`internal/render/readme.go` builds the whole document in memory.
* Render into a `bufio.Writer` so a list with tens of thousands of
  repositories does not need the whole document in memory at once.

### Verify that the tag matches the changelog
The release workflow accepts any `v*.*.*` tag.
* Add a check that the tag matches the version in the release pull request,
  and that the previous tag exists (a first release needs an explicit
  opt-in).

### A reusable workflow for consumers
* Publish the CI workflow as a reusable workflow so that a consumer can call
  it instead of copying the steps.

---

## Done

### The productionisation of the project (#84)

* The findings of the security audit: the validation of the configuration,
  the escaping of the metadata of a repository, the timeouts and the limits
  of the HTTP client, the pinning of the actions and of the base image, and
  the atomic write of the output.
* The findings of the bug hunt: the dead configuration fields, the items
  that were dropped silently, the curation order that was discarded, the
  pagination that always matched, and the explicit zeroes that were
  rewritten by the defaults.
* The continuous integration: `gofmt`, `go vet`, `golangci-lint`, the test
  matrix on Linux, macOS and Windows, the cross compilation, the container
  build and the scan for the known vulnerabilities.
* The release pipeline: GoReleaser, the SBOM, the signatures, the
  provenance attestation, the draft that is published when everything is in
  place, and the container tags that move after the release.
* The command line interface: `generate`, `validate`, `schema` and
  `version`, the `--dry-run` and the `--verbose` flags, and the build
  metadata in the binary.
* The documentation: the README was rewritten from the source, and
  `CONTRIBUTING.md`, `SECURITY.md` and this file were added.

### The individual items of the backlog

* The README claimed the wrong Go version and pointed at the wrong badge.
* `example/jsonschema.json` was a stale duplicate of the real schema - the
  file is gone and the schema is generated into `config-schema.json`.
* `internal/action` was an empty stub - the package is gone.
* The CLI had exactly one flag - it has a subcommand per task now.
* `internal/render/helpers.go` carried a `slug` function that was used
  nowhere.
* `.dockerignore` did not exclude the build output.
