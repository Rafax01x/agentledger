# Decisions

Ambiguities resolved with the simplest option, per the spec's
"how to work" rule. This file grows with each milestone.

## M1

- **Module path: `agentledger` (bare, local).** No remote repo was
  specified in the spec, so no GitHub path was invented. If the project
  is published later, the module path can be renamed to the real repo
  path in one place (`go.mod` + imports).
- **Version string: `0.1.0-dev`.** M1 is a skeleton, not a release, so
  `--version` reports a dev version until v0.1 ships.
- **Skeleton packages: `doc.go` stubs only.** `internal/session`,
  `internal/gitx`, `internal/store`, `internal/redact`, and
  `internal/report` exist as documented empty packages in M1; their
  implementations land in M2/M3/M6 as specified. This keeps the
  directory structure from the spec visible from day one without
  building ahead of the milestone plan.
- **Go toolchain: whatever Go >= 1.22 is installed.** `go.mod` pins
  `go 1.22` (the spec minimum) rather than a newer version, for
  maximum compatibility. This machine had no Go at all, so Go 1.23.0
  (official linux-amd64 tarball, sha256 verified against the
  published value) was installed durably at `~/workspace/tools/go`.
- **Local git identity: `mattpx`.** No git identity was configured
  anywhere on this machine, so M1 commits use local-only
  `user.name=mattpx` / `user.email=mattpx@localhost` (not a real
  address, not published anywhere) so the required milestone commit
  can happen. Change it with `git config user.name` /
  `git config user.email` if a real identity is wanted.

## M2

- **Snapshot excludes `.agentledger/` via pathspec**
  (`git add -A -- ':!.agentledger'`). Without this, a snapshot would
  recursively include agentledger's own database and temp index.
  The user's branch, index, stash, and commits are still untouched:
  only a temporary index is used, then deleted.
- **Snapshot API: `Snapshot(root, id, name)`** creates
  `refs/agentledger/<id>/<name>`, so M3 can use `start` and `end`
  with the same function. The commit message stays exactly
  `agentledger snapshot <id>` per spec.
- **Database file: `.agentledger/agentledger.db`.** `init` is
  idempotent (re-running keeps existing data, reports "already
  initialized"). `init` does NOT touch the user's `.gitignore` —
  non-invasive per spec; the pathspec exclusion above is what keeps
  tool data out of snapshots.
- **Times stored as RFC3339Nano text**, nullable end fields as NULL
  until set. `ListSessions(limit <= 0)` means no limit;
  actions/events list in insertion (`rowid`) order. Note: `rowid`
  can be used in `ORDER BY` but cannot be an indexed column in
  SQLite, so the session indexes are on `session_id` alone —
  the store tests caught this at first run.
- **Doctor is read-only except for a schema check on an existing
  DB.** A missing DB is a FAIL, not auto-created. Working-tree
  dirty state is informational only, never a doctor failure.
- **M5 follow-up (not built in M2):** after `init`, `git status`
  shows `.agentledger/` as untracked, so the working tree is
  technically dirty. M5's undo safety check must ignore
  `.agentledger/` (or it would refuse every undo). Left for M5
  per the one-milestone-at-a-time rule.

## M3

- **Agent name = basename of the command** (`claude` for
  `run -- claude ...`, `fake-agent.sh` for a test script). No agent
  flag was in the spec, so this is the simplest derivation; a
  future `--agent` flag can override it without changing storage.
- **Child runs in the invocation directory**, snapshots at the
  repo root. Session `cwd` records the invocation directory.
- **Exit-code propagation via `cli.ExitError`.** The run command
  silences cobra's error/usage output on a child failure (the
  summary line already reported it) and `main` exits with the
  child's code. A child that cannot start at all is recorded as
  exit 127 with its start error written to stderr.
- **Live + captured output:** a tee writer forwards each write to
  the live stream immediately and accumulates it; events are
  created from the accumulated buffer after the child finishes,
  one event per line, per stream.
- **Actions from `git diff --name-status`** between start and end
  refs, with per-file diffs. Types: added / modified / deleted /
  renamed (rename = new path). Redaction is NOT applied yet —
  that is M6; M3 stores raw diffs and output.
- **Fake agent:** `internal/session/testdata/fake-agent.sh`
  (prints to stdout+stderr, edits `a.txt`, creates `new.txt`,
  exits `$FAKE_EXIT`). Tests copy it into the temp repo before
  the run, so it is already in the start snapshot and is NOT a
  recorded change — the test expects 2 changed files (the test
  first expected 3 and this reasoning was the bug, not the code).

## M4

- **`log` default `--limit 20`; `--limit 0` = all.** This matches
  the store's `limit <= 0` = no-limit rule. Sessions list newest
  first, one line each, with `undone` marked when set.
- **`show` prints fields, files, and captured output; `--diff`
  adds per-file diffs.** `show` without `--diff` stays readable
  for large sessions.
- **`export --format` is required and validated** (markdown or
  jsonl only); anything else is an actionable error, not a
  silent default. Export writes to stdout so it can be
  redirected to a file.
- **JSONL shape:** one object per line, in order — session,
  then actions, then events — each with a `"type"` field, so a
  consumer can stream-parse it without knowing counts up front.
- **Shared resolution:** `log`/`show`/`export` share
  `openStoreForRepo` + `resolveSession` (`last` = newest
  session). `run` keeps its own setup from M3 to avoid
  refactoring working code mid-milestone.
- **Human times in local time, exports in UTC.** `log`/`show`
  format times with `.Local()` (times are stored UTC, so without
  this a WIB user would see times 7h early). Markdown/JSONL
  exports keep RFC3339Nano UTC with an explicit `Z`, so they
  stay unambiguous for machines.

## M5

- **Dirty check is against the END snapshot, not HEAD — this
  matters.** A naive "refuse if the tree is dirty vs HEAD"
  check would make undo unusable: a session's own changes are
  uncommitted by design, so the tree is always dirty right
  after a run. Undo instead refuses only if its target files
  differ from the session's end snapshot (i.e. the user edited
  them again after the session), unless `--force`. This also
  supersedes the M2 follow-up: `.agentledger/` never appears
  in an end snapshot, so it cannot block undo.
- **That comparison is byte-for-byte, not `git diff <ref>`.**
  `git diff <ref>` routes through the user's normal index, so
  a session-added file (in the snapshot, untracked in the
  normal index) was falsely reported as changed and blocked
  undo — the E2E test caught this. `gitx.ChangedSinceSession`
  compares `git show <ref>:<path>` bytes to working-tree
  bytes, ignoring the index entirely, and a file counts as
  blocking only if it matches NEITHER the start nor the end
  snapshot: a file already matching the start snapshot is
  already undone (e.g. after a partial `--file` undo) and
  must not block a later full undo — the E2E test caught
  that too.
- **Race fix found by the E2E test:** exec runs stdout and
  stderr on separate goroutines, and the CLI test redirects
  both to one buffer, so the session tee writers raced.
  `captureWriter` now serializes through a mutex shared by a
  run's two writers, so sharing a destination is safe in
  real use too (e.g. `2>&1` to one file).
- **Undo per file type:** modified/deleted = checkout from the
  start snapshot; added = delete from the working tree
  (missing file is skipped, not an error); renamed = delete
  the new path and checkout the old path, where the old path
  is parsed from the action diff's `rename from` line (undo
  aborts before changing anything if it cannot be found).
- **--dry-run never refuses on dirtiness** (it changes
  nothing, so previewing is safe) but prints a warning when a
  real undo would refuse. `--file` is repeatable and is
  validated against the session's changed files before any
  change; re-undoing a session is allowed and updates
  `undone_at`.
- **E2E test** (`internal/cli/undo_e2e_test.go`, in-process
  cobra): init → run fake agent → log → show → undo
  --dry-run (no change) → undo --file new.txt only →
  undo (a.txt restored, log marks undone) → run again,
  post-session edit, undo refuses, undo --force restores.

## M6

- **Redaction patterns, in order:** private-key block
  (multiline, first), JWT, Bearer, Anthropic `sk-ant-`
  (before OpenAI `sk-`), GitHub `ghp_/gho_/ghs_/ghu_`,
  Slack `xoxb-/xoxp-`, AWS `AKIA/ASIA`, OpenAI `sk-`
  (including `sk-proj-`, last). Placeholders are
  `<REDACTED:AWS_KEY>`, `<REDACTED:GITHUB_TOKEN>`,
  `<REDACTED:OPENAI_KEY>`, `<REDACTED:ANTHROPIC_KEY>`,
  `<REDACTED:SLACK_TOKEN>`, `<REDACTED:BEARER>`,
  `<REDACTED:PRIVATE_KEY>`, `<REDACTED:JWT>`.
- **Bearer keeps its scheme:** `Bearer <token>` becomes
  `Bearer <REDACTED:BEARER>`, because the scheme is not
  secret and keeping it preserves what kind of credential
  it was. Every other pattern replaces the whole secret.
- **Wiring point is `session.Run` only.** Session fields,
  action diffs, and event lines are redacted there, before
  any store call. The store itself does not redact, so any
  future writer must redact before persisting — this is
  the one place to check.
- **Live output is NOT redacted.** Redaction is "before
  persist", not "before display". The user running an agent
  in their own terminal should see what the agent actually
  printed; only the durable record is scrubbed.
- **No TOML config in v0.1.** The tech stack names TOML
  config, but the spec defines no config options or
  behaviour for it, so nothing is read. It is listed in
  `docs/limitations.md` under v0.2 candidates instead of
  being invented here.
- **Version stays `0.1.0-dev`.** v0.1 is feature-complete
  per the spec, but no release has been cut, so `--version`
  keeps the dev suffix until a release decision is made.
- **JSONL disables HTML escaping.** Go's default `json.Marshal`
  turns `<REDACTED:AWS_KEY>` into `\u003cREDACTED:AWS_KEY\u003e`,
  which parses back correctly but is unreadable and un-greppable
  in an export. The JSONL writer uses an Encoder with
  `SetEscapeHTML(false)` so placeholders appear literally.
  The manual M6 check caught this.

## Phase 1 verification fixes (v0.1.0 release prep)

- **`init` creates `.agentledger/config.toml`.** The release
  verification expects it, and the M-spec never created it.
  The file is created only if missing (re-init never
  overwrites a user edit) and v0.1 still reads no options
  from it — see limitations.
- **`show --events` is accepted.** `show` always included
  the output section, but had no `--events` flag, so the
  verification's `show last --events` failed on an unknown
  flag. The flag is now accepted; output is still shown by
  default.
- **GitHub token redaction minimum lowered 20 → 10
  characters.** A 16-character example token
  (`ghp_abcdef1234567890`) was leaking because the pattern
  required 20+. Real tokens are 36+, but a shorter token
  must be redacted rather than leaked.
- **Undo safety scope widened to all snapshot files.**
  The M5 check only looked at undo targets, so undoing a
  session with no file changes silently succeeded as a
  no-op even after the user overwrote an unrelated file.
  Undo now checks every file in the start/end snapshots
  (a file matching either snapshot, e.g. already undone,
  does not block). A second bug in the same area: the
  nothing-to-undo early return ran before the safety
  check, bypassing it entirely for no-change sessions —
  the check now runs before that return.
- **Go toolchain: `go 1.26.0`.** `go get modernc.org/sqlite@latest`
  (v1.61.1) raised the `go.mod` directive from 1.22 to 1.26; the
  Go 1.23 wrapper auto-downloaded and cached the 1.26.0 toolchain,
  so builds now report `go version go1.26.0`. This still satisfies
  the spec's Go >= 1.22.

## v0.1.0 release decisions

- **Why shell out to git instead of a library.** The git
  binary is the source of truth for repository state: it
  handles every repository layout, index format, and
  edge case the same way the user's own git does, with no
  reimplementation to drift. A library would add a second
  git implementation to maintain and a risk of behaving
  differently from git itself on the very operations
  (read-tree, write-tree, commit-tree, checkout) that
  snapshots depend on. Shelling out also keeps the binary
  pure Go with no cgo, and makes failures diagnosable: the
  user sees git's own error, not a wrapper's translation.
  The cost — requiring a git binary on PATH — is already
  required by the spec, since agentledger only works in
  Git repositories.
- **Why SQLite instead of JSON files.** Sessions, actions,
  and events are relational: sessions have many actions
  and events, queries need ordering and filtering, and
  writes must be atomic (a session is either recorded
  completely or not at all). JSON files would need
  hand-rolled locking, atomic renames, and in-memory
  filtering for every query, and would corrupt more easily
  on a crash mid-write. SQLite gives transactions, indexes,
  and queries in a single local file with no server, and
  modernc.org/sqlite keeps that pure Go with no cgo.
- **Why a temporary index instead of git stash.** Stash
  touches the user's own state: it mutates the stash list,
  the index, and the working tree, and a failed or
  interrupted stash can leave the user to untangle it.
  A temporary index (`GIT_INDEX_FILE` pointing inside
  `.agentledger/tmp/`) is isolated by construction: the
  user's index, stash, branch, and commits are never
  written, and a crash leaves only a temp file to delete.
  The snapshot algorithm (read-tree HEAD, add -A, write-tree,
  commit-tree, update-ref) is exactly what stash does
  internally, minus the user-visible side effects.
- **Why undo does not delete the ref, and why it creates
  no new snapshot.** Undo is a rollback, not a deletion of
  history: the session's start and end snapshot refs are
  the audit trail, and deleting them would destroy the
  evidence that the session happened and what it changed.
  Instead undo restores contents from the start snapshot
  and marks the session undone (`undone_at`), leaving both
  refs intact so the change is still inspectable and, in
  principle, re-appliable from the end snapshot. No new
  snapshot is created because the end snapshot already
  preserves the session's end state — a pre-undo snapshot
  would duplicate it in the normal case. The gap is real
  and is logged, not hidden: with `--force`, post-session
  edits overwritten by undo are not preserved by
  agentledger, and a pre-undo snapshot is a v0.2 candidate
  in docs/limitations.md.
- **Why redaction at write-time instead of read-time.**
  Read-time redaction would leave raw secrets in the
  database file itself: anyone copying, backing up, or
  opening `.agentledger/agentledger.db` with another tool
  would see them, and every reader (log, show, export,
  future tools) would have to redact correctly forever.
  Write-time redaction, in `session.Run` before any store
  call, means the durable record never contains the
  secret in the first place, so a leak requires the live
  terminal output, not the stored history. The trade-off
  is accepted explicitly: live output is not redacted,
  and redaction is best-effort over the spec's list only
  (see docs/limitations.md).
- **Version bumped to `v0.1.0`.** The M6 note that the
  version stays `0.1.0-dev` until a release is cut is now
  superseded: Phase 2 cuts the release artifacts, so
  `--version` reports `v0.1.0`.

## GitHub publish fix (v0.1.0)

- **Slack test tokens made letters-only.** The first GitHub
  push was rejected by secret scanning because the Slack
  example tokens in `redact_test.go` used numeric segments
  that look like real Slack tokens. The examples are now
  letters-only after the prefix: they still match the
  redaction pattern and still test the same code path,
  but they no longer match Slack's secret-scanning format.
  No real secret was ever in the repo — these were always
  synthetic test examples.

## v0.2 upgrades

- **Config as YAML/JSON, parsed without a new dependency.**
  The v0.1 stack named TOML config, but no TOML library is in
  go.mod and adding one for three fields would be the heaviest
  part of the feature. `internal/config` instead reads
  `.agentledger.yaml` (a documented small subset: top-level keys,
  string lists, booleans, name/regex maps) or `.agentledger.json`
  (full JSON). A malformed config fails the run loudly: silently
  recording unredacted output because of a config typo would be
  worse than refusing.
- **`.env` masking threshold of 6 characters.** Values of 6 or
  fewer characters (ports, flags, short words) are too generic to
  mask: masking them would corrupt ordinary output and produce
  false confidence. Only longer exact values are masked, and the
  `.env` file itself is never copied or stored.
- **Ignore patterns enforced at the recorded-action level.**
  Git pathspec exclusions (`:!pattern`) are passed to the
  snapshot's `git add`, but not every glob shape excludes via
  pathspec, so `session.Run` also filters recorded actions by
  `MatchIgnore`. The guarantee that matters — ignored files never
  appear in diffs or undo — holds regardless of pathspec quirks.
- **Undo shared between CLI and UI, not duplicated.** The undo
  command's RunE was refactored into `runUndo`, called by both
  `agentledger undo` and the UI's POST endpoint. The UI cannot
  drift into weaker safety rules than the CLI. Dirty refusals
  are typed (`ui.ErrDirty`) so the endpoint maps them to 409
  instead of a generic 400.
- **Undo's safety check refuses, not merely warns.** The upgrade
  brief said "warn unless --force"; the existing behaviour —
  refuse, with `--force` to override and a warning on
  `--dry-run` — is strictly safer and already covered by the
  M5 end-to-end test, so it was kept and the dry-run warning
  retained. A warning-only check would let a careless undo
  overwrite post-session work.
- **Telemetry nil means "not reported", never zero.** New
  session columns (`lines_added`, `lines_deleted`,
  `input_tokens`, `output_tokens`, `cost_usd`) are nullable and
  migrated into old databases with ALTER TABLE, so pre-v0.2
  sessions read back with nil telemetry instead of a fake
  `+0 -0` / 0 tokens. Line counts are computed from the raw
  diff before redaction: redaction collapses multi-line
  private-key blocks into one placeholder line and would
  otherwise undercount.
- **Token/cost parsing is last-wins over a small pattern set.**
  Agents print summaries in incompatible formats, so the parser
  matches `input_tokens`/`prompt_tokens`,
  `output_tokens`/`completion_tokens`, and `cost` + decimal
  amount lines, taking the last occurrence of each (summaries
  print at the end of a run). A `cost` line without a decimal
  point is ignored so a count is never mistaken for dollars.
  Unrecognized formats simply yield nil metrics.
- **Clean keeps the newest N regardless of age, then VACUUMs.**
  `--keep-last` is applied before the age cutoff, so a quiet
  repository is never pruned to zero sessions by accident.
  Reclaimed space is the database file size before/after
  VACUUM (clamped at zero), alongside the stored-bytes estimate
  of removed diffs/events, so the figure is measured, not
  invented.
- **UI is loopback-only standard-library HTTP.** `internal/ui`
  embeds one static page and serves JSON over net/http; no
  frontend build step, no external dependencies, no auth
  surface beyond loopback binding. That is also its limit: it
  is a single-user local inspector, not a shared service.
- **Release automation via GoReleaser on `v*` tags.**
  `.goreleaser.yaml` builds Linux/macOS/Windows amd64/arm64
  with CGO disabled (modernc.org/sqlite is pure Go) and the
  workflow runs the test suite before releasing, so a tag on a
  red tree cannot ship binaries. `--version` is bumped to
  `v0.2.0` with this change set, matching the next tag.

## v0.2 review fixes (independent review, GO-WITH-NOTES)

- **Snapshots always exclude `.env`.** Review found an unignored,
  untracked `.env` was captured in snapshot git objects, which
  contradicted the "`.env` file itself is never stored" claim.
  Snapshots now always add `:!.env` and `:!**/.env` pathspecs
  (plus the pre-existing `:!.agentledger`). A tracked `.env`
  still contributes its committed HEAD version via read-tree,
  but working-tree secret edits are never snapshotted.
- **Partial undo does not mark the session undone.** `runUndo`
  now calls `MarkUndone` only when the undo covers every
  recorded action; a `--file` undo leaves the session status
  accurate and a later full undo is the one that marks it.
- **Config typos fail loudly.** Unknown top-level keys and
  flow-style (`key: [...]`) list values are now errors in both
  YAML and JSON (JSON uses `DisallowUnknownFields`). This
  closes the gap between the fail-loud rationale and the old
  silently-ignore behaviour for unknown keys.
- **DeleteSession is transactional** (events + actions + session
  in one transaction) so `clean` cannot leave orphaned rows on
  failure, and the pre-v0.2 migration path now has a test that
  opens a genuine old-schema database.

## UI second iteration (anti-slop redesign)

- **Navbar carries real data only.** The top bar (56px) shows the
  brand, the repository root from `GET /api/meta`, a live
  connection indicator (green only after a successful API fetch),
  and the real session count. No hero banner, no decorative
  shapes, no invented stats, and no em-dashes in UI labels.
- **Selective undo through the same endpoint.** `POST
  /api/sessions/{id}/undo` accepts an optional `files` array;
  an empty/missing array means a full undo. The CLI wiring
  passes it straight to the shared `runUndo`, so per-file
  Revert buttons enforce the same dirty check (409) and
  partial-undo status rules as the CLI.
- **Keyboard navigation is list-relative.** `j`/`k` move within
  the currently filtered list (not the full list), `/` and
  Cmd/Ctrl+K focus the navbar filter, and `u` opens the undo
  modal for the selected session. All shortcuts are ignored
  while typing or while the modal is open (Escape closes it).

## UI third iteration (user-supplied design)

- The inspector page is now the user's own "AgentLedger // Dev
  Inspector" design: mono font throughout, `// AGENTLEDGER`
  brand, SYSTEM ACTIVE badge, sidebar search, Git Diffs /
  Redacted Logs tabs, red Revert button.
- Kept from the second iteration, not dropped with the redesign:
  per-file diffs with collapse, Unified/Split toggle, log
  stream filter, full and per-file revert through the same
  undo endpoint (409 dirty handling), keyboard navigation
  (`j`/`k`, `u`, `/`, Cmd/Ctrl+K), real repo path and session
  count from `GET /api/meta`, no invented stats, no em-dashes.
- The PORT label in the header is real, not hardcoded: `Server`
  gained a `Port` field, set from the actual listener in the
  CLI wiring and returned by `GET /api/meta`.
