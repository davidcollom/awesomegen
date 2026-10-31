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
pull request.** A pull request that fixes two issues is two pull requests.

---

## Grade the issue before you start

Not every issue is worth the same day. Grade the issue you picked with the
same scale the reviewers use, and say the grade in the pull request body:

| Grade | Meaning                                                                    |
| ----- | -------------------------------------------------------------------------- |
| **P0** | correctness, security or shipping: data loss, a wrong file, a broken build |
| **P1** | a real gap: a silent failure, a missing check, a documented command that does not work |
| **P2** | friction: a confusing message, an ignored flag, a rejected valid file     |
| **P3** | polish: wording, cosmetics, a nicety                                      |

A **P0** goes first, even when it is harder than the rest of the batch. When
a batch is picked, work on one P0 and one or two P2 items, not on five P3
items.

---

## Before you open the pull request

Run the whole suite. `make` runs these targets, but the underlying commands
are the ones that matter:

```bash
gofmt -l .                                              # must print nothing
go vet ./...
go test ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run
go generate ./... && git diff --exit-code -- config-schema.json
```

The last one only matters when the configuration types change: the schema is
generated, so the regenerated file belongs into the same commit.

Then check these four things:

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
   explains the change and ends with `Fixes #102`.

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

The findings of a pull request are read from the API of GitHub:

```bash
gh api repos/davidcollom/awesomegen/pulls/<pr>/comments
```

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

* the whole suite is green,
* the change is covered by a test that fails without it,
* every P0, P1 and P2 finding of the review is fixed in the pull request,
* every P3 finding of the review is deferred with an issue that is linked
  from the pull request body,
* every finding has an answer,
* the body says what changed, why, and how it was tested,
* the issue is closed by the pull request.

---

## The local environment

The Go toolchain is not on the `PATH` by default in the agent sandbox:

```bash
export PATH=$PATH:/home/opencode/sdk/go/bin   # go1.27.3
```

`make` is not installed there either, so run the commands of the targets
directly (the list at the top of this file). Everything else - the linter,
the schema generator - is pinned and runs through `go run`, so no tool has
to be installed.
