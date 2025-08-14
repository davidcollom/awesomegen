# awesomegen

[![Go Version](https://img.shields.io/github/go-mod/go-version/davidcollom/awesomegen)](go.mod)
[![Build Status](https://img.shields.io/github/actions/workflow/status/davidcollom/awesomegen/release.yml?branch=main)](https://github.com/davidcollom/awesomegen/actions)
[![License](https://img.shields.io/github/license/davidcollom/awesomegen)](LICENSE)

**awesomegen** is a Go 1.23 CLI and GitHub Action that generates [Awesome List](https://awesome.re)–style `README.md` files from your [GitHub Star Lists](https://docs.github.com/en/account-and-profile/starring-and-unstarring-repositories/organizing-stars-with-lists).

It works by:

1. **Scraping** your public Star List page(s) to collect `owner/repo` slugs.
2. **Enriching** them with GitHub API data (stars, license, topics, last push, archived flag).
3. **Filtering** based on rules you set (min stars, staleness, archived).
4. **Rendering** to a clean, Awesome-formatted `README.md`.

---

## Features

* 🔍 Pulls data from **public GitHub Star Lists** — no manual curation required.
* 🛠 Configurable filters: `min_stars`, `stale_months`, exclude archived.
* 🗂 Supports multiple lists per config file (one README per list).
* 🧰 Works as a standalone CLI *and* a GitHub Action.
* 📦 Ships with a template repo for quickly spinning up new Awesome lists.
* 🧪 Fully testable with fakes, HTML fixtures, and clockwork time control.

---

## Quick start

### 1. Install the CLI

```bash
go install github.com/davidcollom/awesomegen/cmd/awesomegen@latest
```

### 2. Create a config

```yaml
# config.yaml
version: 1
user: "davidcollom"
lists:
  - slug: "kubernetes-cost-optimisation"
    title: "Awesome Kubernetes Cost Optimisation"
    tagline: "Tools & patterns for cost-efficient Kubernetes."
    output: "README.md"
    min_stars: 50
    stale_months: 24
    badges:
      - "https://img.shields.io/badge/awesome-yes-brightgreen"
```

The `slug` is taken from your list’s URL:

```
https://github.com/stars/<user>/lists/<slug>
```

### 3. Run it locally

```bash
export GITHUB_TOKEN=<your_token>  # increases API limits
aesomegen --config config.yaml
```

This will scrape your Star List, enrich via API, filter, and write `README.md`.

---

## Using as a GitHub Action

The easiest way is to create a **data repo** from the [awesome-list-template](https://github.com/davidcollom/awesome-list-template) and commit your `config.yaml`.

Add this workflow:

```yaml
name: Update Awesome
on:
  schedule:
    - cron: "0 6 * * 1"   # Mondays 06:00 UTC
  workflow_dispatch:
  push:

jobs:
  gen:
    permissions:
      contents: write
      pull-requests: write
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run AwesomeGen
        uses: davidcollom/awesomegen@v1
        with:
          config: config.yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      - name: Create PR
        uses: peter-evans/create-pull-request@v6
        with:
          commit-message: "chore: auto-update README"
          title: "chore: auto-update README"
          branch: "auto/update-readme"
          body: "Automated README refresh."
```

---

## Configuration Reference

| Field          | Type   | Description                                                                 |
| -------------- | ------ | --------------------------------------------------------------------------- |
| `version`      | int    | Config schema version (currently `1`).                                      |
| `user`         | string | Your GitHub username.                                                       |
| `lists`        | array  | One or more list definitions.                                               |
| `slug`         | string | List slug from the URL.                                                     |
| `title`        | string | Title for the Awesome list README.                                          |
| `tagline`      | string | Short description tagline.                                                  |
| `output`       | string | Output file path (default `README.md`).                                     |
| `min_stars`    | int    | Minimum GitHub stars for inclusion.                                         |
| `stale_months` | int    | Exclude repos not pushed to in this many months.                            |
| `badges`       | array  | Markdown image URLs for badges.                                             |
| `categories`   | array  | (Optional) Manually override categories. Usually auto-set to “Repositories” |

---

## Example Output

```markdown
# Awesome Kubernetes Cost Optimisation

![badge](https://img.shields.io/badge/awesome-yes-brightgreen)

> Tools & patterns for cost-efficient Kubernetes.

## Table of Contents
- [Repositories](#repositories)

## Repositories

- [opencost/opencost](https://github.com/opencost/opencost) — ⭐ 4500 · Apache-2.0 · `kubernetes`, `cost` — Open metrics-based cost monitoring
- [kubecost/cost-model](https://github.com/kubecost/cost-model) — ⭐ 3000 · Apache-2.0 — Core cost calculation engine
```

---

## Development

```bash
# Run tests
go test ./...

# Lint (example with golangci-lint)
golangci-lint run
```

---

## License

[MIT](LICENSE)
