# Contributing to agentledger

Thanks for helping improve agentledger. This guide covers
building, testing, and submitting a pull request. One page,
no ceremony beyond what keeps the tool trustworthy.

## Prerequisites

- Go >= 1.22 (the module currently requires Go 1.26 via
  modernc.org/sqlite; the Go wrapper will fetch the toolchain
  if needed).
- A `git` binary on PATH. agentledger shells out to git;
  there is no Git library to install.
- A Git repository to test against. Never test `run` or
  `undo` in a repository with work you care about losing —
  use a throwaway repo (for example `/tmp/al-test`) for
  manual end-to-end checks.

## Build

```sh
make build   # builds ./agentledger, or: go build ./...
```

The binary is a single static Go binary. Cross-compiling
uses `CGO_ENABLED=0` with `-ldflags="-s -w"` (see the
release build steps in the README's install section).

## Test

```sh
make test    # go test ./... -race
make vet     # go vet ./...
```

Both must pass before a pull request. Write tests for new
behaviour in the package that owns it: `internal/gitx` for
git operations, `internal/store` for SQLite, `internal/session`
for run-wrap-record, `internal/redact` for redaction,
`internal/report` for log/show/export output, and
`internal/cli` for end-to-end command behaviour.

Code layout rule, from the original spec: `gitx` is the only
package that calls git, and `store` is the only package that
calls SQL. User-facing strings live in
`internal/cli/messages.go`. Keep those boundaries.

## How to submit a pull request

1. Fork the repository and create a branch from `main`.
2. Make one focused change. Do not mix a feature, a
   refactor, and a fix in one pull request.
3. Run `make vet` and `make test` and confirm both pass.
4. Commit with Conventional Commits
   (`feat:`, `fix:`, `docs:`, `chore:`, and so on).
5. Open a pull request describing what changed, why, and
   what you tested. If the change affects behaviour, note
   the manual end-to-end check you ran (init, run, log,
   show, undo) in a throwaway repo.
6. If you are unsure about a design choice, log it in
   `docs/decisions.md` with the change, choosing the
   simplest option that fits the spec.

## Reporting bugs

Use the bug report issue template. Include the agentledger
version (`agentledger --version`), your OS, your git
version, the exact command you ran, and what you expected
versus what happened. If a secret is involved, redact it
before pasting — never paste a real token into an issue.

## What not to change without discussion

- Never touch the user's branch, index, stash, or commits.
  Snapshots use a temporary index; rollback uses
  `git checkout <ref> -- <paths>`, never `git reset --hard`.
- Never add network calls, telemetry, or cloud features.
  agentledger is local-first by design.
- Never store a secret unredacted. New persisted strings
  must go through `internal/redact` before they reach
  the store.
