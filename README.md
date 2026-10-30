# awesomegen

[![Go Version](https://img.shields.io/github/go-mod/go-version/davidcollom/awesomegen)](go.mod)
[![Build](https://img.shields.io/github/actions/workflow/status/davidcollom/awesomegen/ci.yml?branch=main)](https://github.com/davidcollom/awesomegen/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/davidcollom/awesomegen)](LICENSE)

**awesomegen** is a Go 1.26 CLI and GitHub Action that turns public
[GitHub Star Lists](https://docs.github.com/en/account-and-profile/starring-and-unstarring-repositories/organizing-stars-with-lists)
into [Awesome List](https://awesome.re)-style `README.md` files. You curate the
list by starring repositories on GitHub; awesomegen renders the README, keeps
the curation order and applies the filters you configure.

## How it works

1. **Scrape** - `internal/scrape` reads the HTML of `https://github.com/stars/<user>/lists/<slug>`, follows the pagination and collects the `owner/repo` slugs of the list entries in curation order.
2. **Enrich** - `internal/github` looks every repository up in the GitHub REST API and reads its stars, SPDX licence, description, topics, archived flag and last push date.
3. **Filter** - `internal/render` drops the repositories that are archived, that have fewer stars than `min_stars` or that have not been pushed to within `stale_months`.
4. **Render** - the list is written as Awesome-formatted Markdown: title, badges, tagline, table of contents, one section per category or per topic and one bullet per entry.

## Features

* Reads your public Star Lists - no manual copy and paste of repository names.
* Configurable filters: `min_stars`, `stale_months` and the exclusion of archived repositories.
* Several lists per configuration file, one README per list.
* Curated entries alongside the scraped ones: `categories` and `items` hold hand-picked repositories and external links.
* Two grouping modes: one section per category, or grouping by GitHub topic (`flat` or `nested`).
* Locale-aware star counts (`locale`, `compact` or `none`).
* Runs as a standalone CLI, as a container image and as a composite GitHub Action.
* Safe output handling: the target path is restricted to the directory of the configuration file, the file is written atomically and a list that produces no entries is an error instead of an empty README.
* An offline `validate` command and a `schema` command that prints the JSON schema of the configuration file.

---

## Installation

### CLI

```bash
go install github.com/davidcollom/awesomegen/cmd/awesomegen@latest
```

The module requires Go 1.26 or newer. Released binaries for Linux, macOS and
Windows (`amd64` and `arm64`) are attached to the
[releases](https://github.com/davidcollom/awesomegen/releases).

### Container image

The image is published to `ghcr.io/davidcollom/awesomegen` with the tags
`vX.Y.Z`, `vX`, `vX.Y` and `latest`:

```bash
docker run --rm -v "$PWD:/work" -w /work ghcr.io/davidcollom/awesomegen:v1 --config config.yaml
```

The image runs as the non-root user 65532 (from the distroless base). It
therefore cannot write into a directory that is owned by another user, which
is one of the reasons why the GitHub Action is a composite action and not a
Docker action - see the comment in [action.yml](action.yml).

### GitHub Action

The action downloads the released binary, verifies it against the published
checksums and runs it on the runner. As a step of a workflow it looks like
this:

```yaml
      - uses: actions/checkout@v4
      - uses: davidcollom/awesomegen@v1
        with:
          config: config.yaml
```

The full reference, including a complete workflow, the inputs and the
permissions the workflow needs, is in
[GitHub Action reference](#github-action-reference).

---

## Quick start

### 1. Write a configuration file

```yaml
# config.yaml
version: 1

lists:
  - slug: "kubernetes-cost-optimisation"
    title: "Awesome Kubernetes Cost Optimisation"
    tagline: "Tools and patterns for cost-efficient Kubernetes."
    output: "README.md"
    min_stars: 50
    stale_months: 24
    badges:
      - "https://img.shields.io/badge/awesome-yes-brightgreen"
    sources:
      - user: "davidcollom"
        slug: "kubernetes-cost-optimisation"
    group_by_topic: false
    topic_grouping_mode: "flat"
    topic_fallback: "misc"
    stars_format: "locale"
    locale: "en-GB"
```

The `slug` is the last path segment of your Star List URL
(`https://github.com/stars/<user>/lists/<slug>`). `user` and `slug` at the top
level of a list are the shorthand for a single source, so `user:
"davidcollom"` plus `slug: "kubernetes-cost-optimisation"` is the same thing
as the `sources:` block above. A smaller example lives in
[example/config.yaml](example/config.yaml).

### 2. Check the configuration

```bash
awesomegen validate --config config.yaml
```

The check is offline: nothing is scraped and no request is sent to GitHub.

### 3. Generate the README

```bash
export GH_TOKEN=<your-token>      # optional, but raises the API rate limit
awesomegen --config config.yaml
```

The README is written next to the configuration file. Use `--dry-run` to print
it to the standard output instead:

```bash
awesomegen --dry-run --config config.yaml
```

---

## Configuration reference

The configuration file is YAML. The JSON schema is committed at the root of
the repository as [config-schema.json](config-schema.json) and is printed by
`awesomegen schema`. Unknown keys are rejected, so a misspelled key is
reported instead of being silently ignored. The schema of the file is version
`1`; `version` may be omitted and any other version is rejected.

### `Config` - the top level of the file

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `version` | integer | `1` | Version of the configuration schema. Only `1` is supported. |
| `user` | string | - | GitHub user or organisation used as the default owner of a source. Required for a list that has no `sources` of its own. |
| `lists` | array of [`List`](#list---one-generated-list) | - | The lists to generate. At least one is required. |

### `List` - one generated list

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `slug` | string | - | Identifier of the list. Used as the legacy source slug and to derive the output file name when `output` is not set. Must match `^[A-Za-z0-9._-]{1,100}$`. |
| `sources` | array of [`Source`](#source---one-star-list-to-scrape) | - | The Star Lists to scrape. When it is empty, `{user, slug}` is used as a single source. |
| `title` | string | - | **Required.** The H1 title of the generated README. |
| `tagline` | string | - | Short description. Rendered as a blockquote below the badges. |
| `output` | string | `README.md` (single list) or `<slug>.md` (several lists) | Path of the generated file, relative to the directory of the configuration file. See [Output paths](#output-paths). |
| `min_stars` | integer | `0` | Minimum number of stars a repository needs to be included. Must not be negative. |
| `stale_months` | integer | `24` | A repository that has not been pushed to within this many months is dropped. `0` disables the staleness check. Must not be negative. |
| `badges` | array of string | - | Image URLs, rendered as badges below the title. |
| `categories` | array of [`Category`](#category---one-section-of-the-readme) | - | Hand-curated sections. The scraped repositories are added to a `Repositories` category; a curated category is never replaced. |
| `group_by_topic` | boolean | `false` | `false` renders one section per category, `true` groups the entries by their GitHub topics. |
| `topic_fallback` | string | `misc` | Name of the group that holds the entries which match none of the ranked topics. |
| `topic_grouping_mode` | string | `flat` | `flat` renders one section per topic, `nested` renders a section per topic with a sub-section per second topic. |
| `stars_format` | string | `locale` | `locale` formats the star count with the locale, `compact` uses `12.3k`, `none` omits the star count. |
| `locale` | string | `en-GB` | BCP-47 tag used for the number formatting and for the title casing of the section headings. An invalid tag is rejected. |
| `top_tags_limit` | integer | `10` | Maximum number of topic groups. `0` means no limit. Must not be negative. |
| `min_tag_count` | integer | `1` | Minimum number of repositories a topic must appear in before it becomes a group. Must not be negative. |
| `tag_aliases` | map of string to string | - | Maps a topic onto a canonical name, for example `{"machine-learning": "ai"}`. The keys and the values are matched case-insensitively. |

The three numeric settings `stale_months`, `top_tags_limit` and `min_tag_count`
are optional pointers. An unset value keeps its default, and an explicit `0`
keeps its own meaning: `stale_months: 0` disables the staleness check,
`top_tags_limit: 0` means "no limit" and `min_tag_count: 0` lets every topic
become a group.

### `Source` - one Star List to scrape

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `user` | string | - | **Required.** GitHub user or organisation that owns the list. |
| `slug` | string | - | **Required.** Slug of the list, the last segment of `/stars/<user>/lists/<slug>`. |

Several sources can be given for one list; their repositories are merged and de-duplicated.

### `Category` - one section of the README

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `name` | string | - | **Required.** Name of the section. Rendered as a level three heading. |
| `items` | array of [`Item`](#item---one-entry-of-a-category) | - | The entries of the section. |

### `Item` - one entry of a category

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `type` | string | inferred | `github` for a repository, `link` for an external link. Inferred from `repo` or `url` when it is omitted. |
| `repo` | string | - | The repository in `owner/repo` format. Required for a `github` item. |
| `url` | string | - | The URL of the link. Required for a `link` item. |
| `title` | string | - | Display title. For a link item the URL is used when no title is given. |
| `notes` | string | - | Additional notes, appended after the description. |

A curated category with a link item:

```yaml
lists:
  - slug: "tools"
    title: "Awesome Tools"
    categories:
      - name: "Reading"
        items:
          - url: "https://go.dev/blog/"
            title: "The Go Blog"
            notes: "Worth reading"
          - repo: "cli/cli"
            notes: "The GitHub CLI"
```

A repository that appears in more than one category is rendered once, in the
first category that holds it.

### Output paths

The `output` path is resolved relative to the directory that holds the
configuration file, so the result does not depend on the working directory.
It has to satisfy all of the following rules:

* it is a relative path - an absolute path is rejected;
* it names a file, not a directory;
* it contains no `..` segment and no hidden segment (a segment that starts with a `.`, which rules out `.github` and dotfiles);
* it uses one of the extensions `.md`, `.markdown` or `.txt`.

The file names that are derived from a `slug` are held to the same rules.
When a configuration file holds more than one list, each list is written to
`<slug>.md` unless it sets an `output` of its own; two lists may never write
to the same file.

---

## CLI reference

```text
awesomegen [flags]          generate the README(s) (same as "awesomegen generate")
awesomegen generate [flags] generate the README(s) from the Star Lists
awesomegen validate [flags] check the configuration file without contacting GitHub
awesomegen schema           print the JSON schema of the configuration file
awesomegen version          print the version of awesomegen
```

Cobra also provides `awesomegen help`, `awesomegen completion`, `--help` and `--version`.

### Global flags

| Flag | Default | Description |
| --- | --- | --- |
| `-c`, `--config` | `config.yaml` | Path of the configuration file. |
| `--verbose` | `false` | Log what is happening in more detail. The messages go to the standard error stream. |
| `--dry-run` | `false` | Print the rendered README(s) to the standard output instead of writing them. The comment above the document names the file it would have been written to. |

### `awesomegen generate`

Scrapes the Star Lists, enriches the repositories, applies the filters and
writes the README files. The root command does the same thing, so
`awesomegen --config config.yaml` and `awesomegen generate` are equivalent.

```bash
awesomegen generate --config config.yaml
awesomegen --dry-run --config config.yaml
```

A repository that cannot be looked up is reported and skipped, so a single
deleted repository does not stop the list from being regenerated. A list that
produces no entries at all is an error: the tool refuses to overwrite a
curated README with an empty one.

### `awesomegen validate`

Parses and validates the configuration with the same loader the generator
uses, resolves the output paths and prints a summary. No request is sent to
GitHub, so it is safe to use in a pre-commit hook or a pull request check.

```console
$ awesomegen validate --config config.yaml
config.yaml is valid
1 list(s)

- kubernetes-cost-optimisation
  title:       Awesome Kubernetes Cost Optimisation
  source:      davidcollom/kubernetes-cost-optimisation
  output:      README.md
  min_stars:   50
  stale:       24 month(s)
  stars:       locale
  topics:      flat
  entries:     0 curated
```

### `awesomegen schema`

Prints the JSON schema of the configuration file. The schema is embedded in
the binary, so the command works offline and prints the same document that is
committed as [config-schema.json](config-schema.json):

```bash
awesomegen schema > config-schema.json
```

### `awesomegen version`

Prints the version, the commit and the build date that were stamped into the
binary, the Go version and the platform. The commit and the date are left out
when the binary was built without them, for example with `go run`:

```console
$ awesomegen version
awesomegen 1.2.3
commit:     6f1c2d0a9b8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a
built:      2026-01-31T09:15:00Z
go:         go1.26.0
platform:   linux/amd64
```

### Authentication

The GitHub API is called with the token from `GH_TOKEN`; `GITHUB_TOKEN` is
used when `GH_TOKEN` is not set. Without a token the API rate limit is low
and a large list is likely to hit it:

```bash
export GH_TOKEN=ghp_...
```

---

## GitHub Action reference

The action is a **composite** action: it downloads the released binary,
verifies it against the published `checksums.txt` and runs it on the runner.
It therefore works on Linux, macOS and Windows runners and needs no
container pull.

### Inputs

| Input | Default | Description |
| --- | --- | --- |
| `config` | `config.yaml` | Path of the configuration file, relative to the workspace. |
| `version` | `latest` | The release to run, for example `v1.2.3`. `latest` resolves the newest published release through the GitHub API. |
| `github-token` | `${{ github.token }}` | Token used for the GitHub API calls. It is passed to the tool as `GH_TOKEN` and `GITHUB_TOKEN`. |

### Example workflow

```yaml
name: Update Awesome
on:
  schedule:
    - cron: "0 6 * * 1"   # Mondays, 06:00 UTC
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  gen:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run AwesomeGen
        uses: davidcollom/awesomegen@v1
        with:
          config: config.yaml
          version: v1.2.3
      - name: Create PR
        uses: peter-evans/create-pull-request@v6
        with:
          commit-message: "chore: auto-update README"
          title: "chore: auto-update README"
          branch: "auto/update-readme"
          body: "Automated README refresh."
```

### Permissions

The action itself only reads the workspace and calls the GitHub API, so the
`contents: read` permission of the default token is enough for that part.
Writing the result back needs more: `contents: write` to commit the generated
file directly, and `pull-requests: write` on top of it to open a pull request.

The `github-token` input defaults to the token of the workflow, so no
personal access token has to be pasted into the workflow file.

---

## Output format

The generated document has a fixed shape: the title, the badges, the
tagline, the table of contents, one section per group and the footer.

```markdown
# Awesome Kubernetes Cost Optimisation

![badge](https://img.shields.io/badge/awesome-yes-brightgreen)

> Tools and patterns for cost-efficient Kubernetes.

## Table of Contents

- [Repositories](#repositories)
- [Reading](#reading)

### Repositories

- [opencost/opencost](https://github.com/opencost/opencost) — ⭐ 4,500 · Apache-2.0 — Open metrics-based cost monitoring for Kubernetes
- [kubecost/cost-model](https://github.com/kubecost/cost-model) — ⭐ 3,000 · Apache-2.0 — Core cost calculation engine

### Reading

- [The Go Blog](https://go.dev/blog/) — Worth reading

---
*Generated by [awesomegen](https://github.com/davidcollom/awesomegen).*
```

A repository entry is written as
`- [<owner/repo>](<url>) — ⭐ <stars> · <licence> — <description> — <notes>`
and a link entry as `- [<title>](<url>) — <notes>`. The parts that are not
available are left out: a repository without a licence has no
`· <licence>` part, `stars_format: none` removes the star count and `notes`
is only appended when the entry has one.

With `group_by_topic: true` the sections are the topics instead of the
categories, and the table of contents links to them. With
`topic_grouping_mode: nested` every topic gets a sub-section for its second
topic:

```markdown
## Table of Contents

- [Kubernetes](#kubernetes)
- [Misc](#misc)

### Kubernetes

#### Cost

- [opencost/opencost](https://github.com/opencost/opencost) — ⭐ 4,500 · Apache-2.0 — Open metrics-based cost monitoring for Kubernetes
```

Details worth knowing:

* The heading anchors are allocated the way GitHub allocates them, so the table of contents and the headings always agree, including for repeated headings.
* The repositories come first in a section, the links second, and the curation order is preserved within both groups.
* An entry that matches none of the ranked topics - and every link, because a link has no topics - ends up in the `topic_fallback` group.
* The star count is formatted with the locale, so `en-GB` and `en-US` render `4,500`, while `de-DE` renders `4.500`.

---

## Development

```bash
git clone https://github.com/davidcollom/awesomegen.git
cd awesomegen
make build
./bin/awesomegen --version
```

The project needs Go 1.26 or newer and nothing else: the linter, GoReleaser
and govulncheck are run through `go run` with a pinned version.

### Make targets

| Target | What it does |
| --- | --- |
| `make help` | list every target |
| `make build` | build the CLI into `bin/` |
| `make install` | install the CLI into the Go bin directory |
| `make test` | run the unit tests |
| `make test-race` | run the unit tests with the race detector (needs cgo) |
| `make cover` | run the tests and write `coverage.out` |
| `make lint` | run golangci-lint |
| `make fmt` | format the source with `gofmt` |
| `make fmt-check` | fail when the source is not formatted |
| `make vet` | run `go vet` |
| `make schema` | regenerate `config-schema.json` from the Go types |
| `make schema-check` | fail when `config-schema.json` is out of date |
| `make tidy` | run `go mod tidy` |
| `make vuln` | run govulncheck against the dependency graph |
| `make release` | cut a release with GoReleaser |
| `make snapshot` | build a snapshot release with GoReleaser (nothing is published) |
| `make clean` | remove `bin/`, `dist/` and `coverage.out` |

### Tests

The tests never touch the network: `make test` runs them and `make test-race`
adds the race detector. The scraper is tested against the HTML fixture in
`testdata/`, served from a local test server, the repository lookups use a
fake client, and the clock is controlled with `clockwork`, so the staleness
filter is tested without waiting for the calendar.

### The JSON schema

`config-schema.json` is generated from the Go types in
`internal/config/types.go`. The generator writes no timestamp, so the output
is reproducible and the drift check does not produce false positives. Run
`make schema` and commit the result; CI and the release pipeline both fail
when the committed schema does not match the generated one.

### Releases

Releases are cut by pushing a `vMAJOR.MINOR.PATCH` tag. The release workflow
builds the binaries for Linux, macOS and Windows on `amd64` and `arm64`,
creates the archives, the checksum file and a per-archive SPDX SBOM, pushes
the multi-architecture image to `ghcr.io/davidcollom/awesomegen`, signs the
artefacts with cosign (keyless, using the OIDC identity of the workflow) and
records a build provenance attestation.

To rehearse a release without publishing anything, run `make snapshot`. The
snapshot version carries a `-next` suffix, so it can never be mistaken for a
published release. The details are in [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Security

Please do not open a public issue for a security problem; see [SECURITY.md](SECURITY.md).

The tool is hardened against a number of failure modes, and the hardening is
covered by tests:

* the output path is restricted to the directory of the configuration file, may not contain hidden segments and must use a Markdown or text extension;
* the tool refuses to write through a symbolic link, and the file is written to a temporary file that is renamed into place, so an interrupted run never leaves a half-written README behind;
* the configuration file and the scraped page are size limited;
* unknown keys in the configuration file are rejected;
* a list that produces no entries is an error, so an empty README is never written over a curated one;
* the text that comes from GitHub - descriptions, licences and topics - is escaped before it is written, and only `http` and `https` URLs are rendered.

---

## Contributing

Contributions are welcome - see [CONTRIBUTING.md](CONTRIBUTING.md) for the
prerequisites, the make targets, the code style and how a release is cut.

## License

[MIT](LICENSE)
