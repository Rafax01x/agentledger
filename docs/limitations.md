# Limitations

What agentledger v0.1 does not do, honestly stated. Non-goals from
the spec (no GUI, no cloud, no AI summaries, no multi-agent
orchestration, no non-Git support, no plugin system) are out of
scope by design and are not repeated here as limitations.

## Known limitations in v0.1

- **Redaction covers the spec's list only.** AWS keys (AKIA/ASIA),
  GitHub `ghp_/gho_/ghs_/ghu_` tokens, OpenAI `sk-` keys,
  Anthropic `sk-ant-` keys, Slack `xoxb-/xoxp-` tokens, Bearer
  tokens, private-key blocks, and JWTs. Other secret shapes
  (for example `github_pat_` fine-grained tokens, npm/PyPI
  tokens, GCP service-account keys, database URLs with
  passwords) are stored as-is. Redaction is best-effort, not
  a security guarantee: do not rely on it as a guarantee
  that no secret can ever reach the database. Review exports
  before sharing them.
- **Live output is not redacted.** Only persisted strings are
  redacted. A secret printed by an agent is still visible in
  the terminal that ran it, and in that terminal's scrollback.
- **Undo is file-content based, not mode/ownership based.**
  Undo restores file contents from the start snapshot; file
  mode changes alone do not block undo, and directory
  structure changes beyond the recorded files are untouched.
  Renamed files undo only when the action diff still contains
  a `rename from` line.
- **Snapshots require a HEAD commit.** `run` in a repo with no
  commits yet fails with an actionable error; commit something
  first, then run.
- **Snapshots require a working git binary and git config.**
  `commit-tree` needs a committer identity, as any git commit
  does. If git is unconfigured, snapshots fail with git's own
  error.
- **Binary files** are recorded as changed, and their diffs
  read "Binary files differ" (git's own output). Undo still
  restores their contents correctly.
- **No config file is read in v0.1.** The tech stack names
  TOML config, but v0.1 defines no config options and reads
  none. See v0.2 candidates below.
- **No Windows path testing done yet.** The code is pure Go
  plus the git binary and should cross-compile, but Windows
  paths, shells, and git behaviour are not verified in v0.1.
  Tested on Linux only.
- **Only tracks files inside the Git working tree.**
  Files outside the repository, or in a different repository,
  are not recorded, and a session cannot span repositories.
- **GUI agents are not supported in v0.1.** Only CLI agents
  run through `agentledger run -- <cmd>` are recorded. An
  agent that edits files through a GUI without a wrapped CLI
  command leaves no agentledger session.
- **Git refs under `refs/agentledger/` accumulate.**
  Every session leaves a start and an end snapshot ref, and
  v0.1 never deletes or prunes them. Garbage collection is a
  v0.2 candidate (see below); until then the ref namespace
  grows with the session count.
- **`.agentledger/` itself is untracked tool data.** It is
  excluded from snapshots and from undo's safety check, but
  it does show up in `git status` as untracked until the user
  chooses to ignore it. `init` deliberately does not modify
  the user's `.gitignore`.

## v0.2 candidates

Features noticed while building v0.1, deliberately NOT built
here, per the spec rule. No work has started on any of these.

- TOML config file support (default `--limit`, default
  export format, redaction on/off, agent aliases).
- `--agent` flag for `run` to override the agent name
  (currently the command basename).
- `log` filtering: by agent, by date range, by exit code,
  full-text search over commands and output.
- `export --output <file>` instead of stdout-only export.
- Session pruning / garbage collection of old
  `refs/agentledger/*` snapshots.
- A pre-undo snapshot: `undo` currently restores from the
  start snapshot without first snapshotting the current
  tree, so post-session edits overwritten with `--force`
  are not preserved by agentledger (the session's end
  snapshot still preserves the session's own end state).
- `doctor --fix` to create a missing `.agentledger/`
  automatically.
- Redaction for additional secret shapes (`github_pat_`,
  npm, PyPI, GCP keys, URLs with embedded passwords),
  and a `--no-redact` escape hatch for local debugging.
- Undo for file-mode-only changes, and undo of an entire
  directory tree as one unit.
- A `run --dry-run` that snapshots without running, for
  testing snapshot behaviour.
- Shell completions and a man page.
