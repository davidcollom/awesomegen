# Security policy

## Supported versions

`awesomegen` is a command line tool and a GitHub Action; there is no long
term support branch. Fixes are released for the latest published version,
and the latest version is what the GitHub Action installs by default.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| older   | :x:                |

If a problem is reported against an older version, it is still fixed - the
fix simply ships in the next release.

---

## How to report a vulnerability

**Do not open a public issue, a discussion or a pull request for a security
problem.**

Report it privately through GitHub's private vulnerability reporting:

1. Open the repository at <https://github.com/davidcollom/awesomegen>.
2. Go to **Security** -> **Advisories** -> **Report a vulnerability**.
3. Describe the problem, including the affected version, the steps to
   reproduce it and the impact you believe it has.

If private advisory reporting is not available to you, open a regular issue
that asks the maintainer to contact you privately, without any technical
details.

Please do not disclose the problem publicly until it has been fixed and a
release has been published.

---

## What to expect

| Step                                        | Target time                      |
| ------------------------------------------- | -------------------------------- |
| Acknowledgement that the report arrived     | 3 working days                   |
| First assessment (valid / not valid, severity) | 7 working days               |
| Fix or mitigation for a confirmed issue     | 30 days, sooner if severe        |
| Release with the fix                        | within 7 days of the fix         |
| Public disclosure                           | together with the release        |

A maintainer may ask for more information, and may ask you to verify the
fix. You will be credited in the advisory and the release notes unless you
prefer to stay anonymous.

If a report is not a security problem, it is usually turned into a normal
issue - you will be told that, and it will be tracked like any other
feedback.

---

## What is in scope

`awesomegen` ships three artefacts, and all three are in scope:

* the `awesomegen` command line binary;
* the `davidcollom/awesomegen` GitHub Action;
* the `ghcr.io/davidcollom/awesomegen` container image.

Typical classes of problem that are in scope:

* writing a file outside of the directory that holds the configuration
  file, or through a symbolic link, including through a value that is
  derived from user supplied data such as a list slug;
* escaping the sandbox the tool is meant to run in, for example by
  injecting content into the generated Markdown that turns into active
  content on GitHub;
* leaking the GitHub token, or any other credential, into the generated
  file or into a log message;
* unbounded resource consumption, for example by feeding a very large
  configuration file or a very large Star List page;
* a vulnerability in the release pipeline: an unsigned or unverifiable
  artefact, a container image that does not contain the released binary, or
  a supply chain compromise through the build.

---

## What is out of scope

* The project scrapes the HTML of the public GitHub Star List pages. When
  GitHub changes that markup the scraper breaks. That is a robustness
  problem, not a vulnerability, and it is reported as a normal issue.
* Findings that require an attacker to already control the machine that
  runs the tool, or that require write access to the repository that is
  being generated.
* Missing hardening in a third-party dependency that is not reachable
  through the code in this repository. Report those to the dependency
  instead - `govulncheck` runs on every pull request and on every push to
  `main`, which is what catches them.
* Denial of service through the public GitHub API rate limit. Use a token
  with the scopes you need and run the tool less often.

---

## Hardening that is already in place

The following protections exist in the current code base and are covered by
tests. They are listed here so that a report is not filed for behaviour that
is intentional:

* Output paths are restricted to relative paths inside the directory that
  holds the configuration file, may not contain hidden path segments and
  must use one of the extensions `.md`, `.markdown` or `.txt`.
* The tool refuses to write through a symbolic link, and the file is written
  to a temporary file that is renamed into place, so an interrupted run
  never leaves a half written README behind.
* The configuration file is size limited, and the scraped Star List page is
  size limited, so a single document cannot exhaust the memory.
* Unknown keys in the configuration file are rejected instead of being
  silently ignored, so a typo cannot quietly change the behaviour.
* A list that produces no entries is an error: an empty README is never
  written over a curated one.
* Third party actions in the workflows are pinned to a full commit SHA, and
  the release artefacts are signed with cosign and covered by a build
  provenance attestation.

---

## Advisories

Published advisories for this repository are listed under
**Security** -> **Advisories** at
<https://github.com/davidcollom/awesomegen/security/advisories>. The
release notes of every release link to the advisory when a fix was made for
a reported problem.
