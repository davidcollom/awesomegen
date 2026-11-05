# AGENTS.md

How an agent works on `awesomegen`. It describes the whole path of a change:
from the issue that describes the work, over the pull request that carries
it, to the review that comes back and the findings that have to be answered.

The rules below exist because they cost a lot of time when they are missed:
a review finding that is ignored comes back on the next pull request, and a
change without a test that fails without it proves nothing.

---

## The work is tracked in the GitHub issues

The backlog lives in the issue tracker, not in a file of the repository. Every
issue names the files it touches, what the change should do and the test that
has to fail without the change.

```bash
gh issue list --repo davidcollom/awesomegen --state open
gh issue view 102 --repo davidcollom/awesomegen
```

Pick the issue, work on it, close it with the pull request. **One issue per
pull request.** A pull request that fixes two issues is two pull requests. A
pull request that fixes nothing at all - a change of the documentation, for
instance - needs no issue.

---

## Grade the work before you start

Not every piece of work is worth the same day. Grade the work you picked
with the scale below - the one the issues of the tracker are graded with -
and say the grade in the pull request body:

| Grade | Meaning                                                                    |
| ----- | -------------------------------------------------------------------------- |
| **P0** | correctness, security or shipping: data loss, a wrong file, a broken build |
| **P1** | a real gap: a silent failure, a missing check, a documented command that does not work |
| **P2** | friction: a confusing message, an ignored flag, a rejected valid file     |
| **P3** | polish: wording, cosmetics, a nicety                                      |

A **P0** goes first, even when it is harder than the rest of the batch. When
a batch is picked, work on one P0 and one or two P2 items, not on five P3
items.

The scale above grades the work itself. A finding of a review carries a grade
of its own - critical, high, medium or low - and that one is described in the
next section.

---

## Before you open the pull request

Run the checks that CI runs. The `make` targets cover most of them, but the
underlying commands are the ones that matter:

```bash
# the lint job
gofmt -l .                                              # must print nothing
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run
go generate ./... && git diff --exit-code HEAD -- config-schema.json

# the test job
go test ./...
CGO_ENABLED=1 go test -race ./...                       # needs a C compiler

# the govulncheck job
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# the platforms the test job runs on
GOOS=windows go vet ./...
GOOS=darwin go vet ./...
```

These are the commands of the `lint`, `test` and `govulncheck` jobs of
`.github/workflows/ci.yml`. The `test` job runs the suite on Linux, macOS
and Windows, and it uses the race detector everywhere but on Windows, where
the image does not guarantee a C compiler. The last two commands check the
two other platforms locally, because a `go test` for them needs a runner of
their own.

The `build` job cross compiles every released target and builds the
container image. The cross compilation is cheap, so run it as well:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/awesomegen
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o /dev/null ./cmd/awesomegen
```

The schema check only matters when the configuration types change: the schema
is generated, so the regenerated file belongs into the same commit. It
compares the file with `HEAD` and not with the index, because `git diff --
<path>` compares the working tree with the index: a schema that was
regenerated and staged would pass the check although the committed file is
stale. That is the defect of #85, which fixes the `make schema-check`
target; the command here and the target have to be fixed together. Run the
check after the regenerated schema is committed - a schema that was
regenerated but not committed yet still fails it.

Then check these five things:

1. **The change is covered by a test that fails without it.** New behaviour
   needs a test, a bug fix needs a regression test. A test that passes
   before the change covers nothing. Prove it: revert the change, run the
   test, watch it fail, restore the change.
2. **The change is focused.** No unrelated reformatting, no drive-by
   renames, no new dependency that is not needed.
3. **The code reads like the code around it.** Errors are wrapped with `%w`
   and carry context, functions are small, and a doc comment explains *why*
   rather than *what*.
4. **The commit message is a Conventional Commit.** `fix(generate): probe
   every name for the case-insensitive file system`, with a body that
   explains the change.
5. **The pull request body states the grade of the work and closes the
   issue.** It starts with `Fixes #102` and says the grade of the work. The
   closing keyword has to be in the body: the repository is squash merged,
   so the commit messages of the branch are discarded at the merge and the
   body of the pull request becomes the body of the squash commit. GitHub
   closes the issue when the keyword survives into that commit, so a
   `Fixes #102` in a commit message alone closes nothing. The commit message
   may repeat the keyword.

---

## After the pull request is open: the review findings

The reviewers post their findings on the pull request as review comments.
Codex labels every finding with a badge, Copilot posts the same kind of
finding without one. Treat a Copilot finding as the grade it would have if
it had a badge.

| Badge | Grade    | What has to happen                                          |
| ----- | -------- | ------------------------------------------------------------ |
| **P0** | critical | fixed in this pull request, before it is merged              |
| **P1** | high     | fixed in this pull request, before it is merged              |
| **P2** | medium   | fixed in this pull request, before it is merged              |
| **P3** | low      | may be deferred, but only with a tracking issue (see below) |

The rule that matters:

> **Every high and medium finding (P0, P1, P2) is resolved in the pull
> request that carries it. A low finding (P3) may be deferred, but only when
> an issue is opened for it.**

### High and medium findings (P0, P1, P2)

1. Read the finding and decide whether it is correct. A finding that does
   not hold is answered with the reason, not with a change.
2. Fix it on the same branch, in the same pull request.
3. Add or extend a test that covers the case the finding describes.
4. Reply to the finding with what changed and the name of the test.
5. Ask for a new review (`@codex review`) and wait for it.

A pull request with an open P0, P1 or P2 finding is **not done**, no matter
how good the change itself is.

### Low findings (P3): defer them, but never lose them

A low finding may be left for later. It may **not** be left without a trace.
Deferring a finding means all of the following:

1. **An issue is opened for it.** The issue names the file and the line, what
   the reviewer said, what has to change and which test covers it. It is a
   normal issue, in the tracker, with the same shape as every other issue.
2. **The pull request body lists it.** The body has a short section that
   names every deferred finding and links to the issue that tracks it, so a
   reviewer sees the deferral without reading the whole thread.
3. **The fix lands in a separate pull request.** The deferral is not a
   reason to pile an unrelated change into the current pull request.

Never resolve a review comment without either a fix in the pull request or
an issue that tracks the finding. A comment that is answered with "fixed
later" and without an issue is a finding that is lost.

### Listing the findings

The findings of a pull request are read from the API of GitHub. Set `PR` to
the number of the pull request and use it in the path:

```bash
PR=127
gh api "repos/davidcollom/awesomegen/pulls/$PR/comments"
gh api "repos/davidcollom/awesomegen/issues/$PR/comments"
```

Write the number as it is. `pulls/<pr>/comments` does not work in a shell:
`<pr>` is an input redirection there, and the command reads
`pulls//comments` instead.

The first command returns the review comments - the findings of the
reviewers, with the file and the line they are about. The second one
returns the conversation comments: the `@codex review` requests and the
review summary of the Codex reviewer.

The badge of the Codex reviewer is in the body of the comment, so the grade
of a finding can be read from it. A finding of Copilot carries no badge:
grade it by hand and treat it as the grade it would have.

### Answering a finding

Every finding gets an answer, even when it is deferred. The answer says
what was done:

```text
Fixed in <commit>: the probe now walks every entry of the directory instead
of stopping at the first one. The new test `TestCaseInsensitiveDirProbesEveryName`
covers the dangling symlink that hid the file system.
```

```text
Deferred as #137: the suggestion is right, but it touches the renderer and
is not related to this change. The issue names the file and the line.
```

---

## The definition of done

A pull request is done when:

* every check of the suite above is green,
* the change is covered by a test that fails without it,
* every P0, P1 and P2 finding of the review is fixed in the pull request,
* every P3 finding of the review is deferred with an issue that is linked
  from the pull request body,
* every finding has an answer,
* the body says what changed, why, and how it was tested,
* the body states the grade of the work,
* the body closes the issue, when the pull request fixes one.

---

## The local environment

The Go toolchain is not on the `PATH` by default in the agent sandbox:

```bash
export PATH=$PATH:/home/opencode/sdk/go/bin   # go1.27.3
```

`make` is not installed there either, so run the commands of the targets
directly (the list in "Before you open the pull request"). Every tool that
is not part of the Go distribution is pinned and runs through `go run`, so
nothing has to be installed: the linter (`golangci-lint` v2.15.0), the
scanner (`govulncheck` v1.9.0) and GoReleaser (v2.19.0). The schema is
generated by the repository itself, with `go generate ./...`.
