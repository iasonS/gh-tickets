# gh-tickets

A GitHub CLI extension that groups open pull requests by ticket. One ticket (PROJ-123) often
needs pull requests in several repositories; GitHub lists them per repository, `gh tickets` per
ticket, one line each with author, branches, draft, review, checks, merge state and age.

## Install

There are no release binaries yet. `gh extension install iasonS/gh-tickets` installs a Go
extension only from release assets named `gh-tickets-<os>-<arch>` (gh never builds Go), so until
a release workflow exists, build it and link the clone (the folder must be named `gh-tickets`):

    git clone https://github.com/iasonS/gh-tickets && cd gh-tickets
    make build                # runs Go in Docker, nothing else to install
    gh extension install .

## Commands

    $ gh tickets --owner acme        # or --repo acme/api --repo acme/infra; --mine; --review
    PROJ-12
      acme/api#41   PROJ-12 Retry webhooks  dana  main ← proj-12-retry         approved  passing  mergeable  2d
      acme/infra#7  Queue for retries       lee   main ← PROJ-12-queue  draft  waiting   running  unknown    5h

    $ gh tickets show PROJ-12        # adds what merged in the last --since (default 30d)
    $ gh tickets stacked             # pull requests based on another pull request's branch
      acme/api#43   Retry tests  dana  proj-12-retry ← proj-12-tests    waiting   running  mergeable  1h
          main ← #41 ← #43
      acme/infra#9  Alarms       lee   old-base ← alarms                approved  passing  mergeable  3d
          old-base ← #9  parent #8 is merged: retarget the base before merging

Without `--owner` or `--repo` it searches the owner of the current repository. A base counts as
stacked unless it is the default branch or in `--release-branches`, by default
`main,master,dev,develop,prod,production`; add your other long-lived branches. Every command
takes `--json`.

## How the ticket key is found

The key is the first match of `--pattern` (default `[A-Z][A-Z0-9]+-\d+`) in the title, else in
the head branch. Pull requests without one go in the last group, "No ticket".

## It only reads

It reads GitHub search with your `gh` login and changes nothing. Search stops at 1000 results;
gh-tickets says when it hits that limit. Colours appear only on a terminal.

## Build

    make test     # go vet and go test, in Docker
    make build    # the binary for this machine
