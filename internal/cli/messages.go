// Package cli holds the cobra commands for agentledger.
//
// All user-facing strings live in this file (messages.go), so wording
// can be reviewed and changed in one place.
package cli

// User-facing strings for the root command and the app as a whole.
const (
	// AppName is the binary / command name shown to users.
	AppName = "agentledger"
	// Tagline is the product tagline.
	Tagline = "Git for AI agent actions."
	// Version is the current version reported by --version (M1 skeleton).
	Version = "v0.1.0"
	// RootShort is the one-line description shown in help.
	RootShort = "Local-first audit trail and rollback CLI for AI coding agents."
	// RootLong is the longer help text for the root command.
	RootLong = `agentledger is a local-first audit trail and rollback CLI for AI coding agents.

It wraps any CLI command, snapshots the Git working tree before and
after, records every file change plus stdout/stderr, and lets you
inspect or undo any session with one command.

Local-first. No network. No telemetry. Agent-agnostic. Non-invasive.`

	// init command.
	InitUse   = "init"
	InitShort = "Initialize agentledger in the current Git repository."
	InitLong  = `Create a .agentledger/ directory in the Git repository root and
initialize its SQLite database. Safe to run again: existing data is kept.`
	InitInitialized        = "Initialized agentledger in %s"
	InitAlreadyInitialized = "agentledger is already initialized in %s"
	InitNotGitRepo         = "not a git repository: %s — run agentledger init inside a git repository (run git init first, then retry)"

	// doctor command.
	DoctorUse   = "doctor"
	DoctorShort = "Run diagnostics on the agentledger setup."
	DoctorLong  = `Check that git is available, the current directory is in a git
repository with a HEAD commit, and that .agentledger/ and its database exist.`
	DoctorCheckGit       = "git binary"
	DoctorCheckRepo      = "git repository"
	DoctorCheckHead      = "HEAD commit"
	DoctorCheckDir       = ".agentledger directory"
	DoctorCheckDB        = "database"
	DoctorOK             = "OK"
	DoctorFail           = "FAIL"
	DoctorAllOK          = "All checks passed."
	DoctorFailed         = "doctor found %d failing check(s) — fix the FAIL items above and run agentledger doctor again"
	DoctorWorkingTree    = "working tree"
	DoctorWorkingClean   = "clean"
	DoctorWorkingDirty   = "dirty (informational only, not a failure)"
	DoctorNotInitialized = ".agentledger/ is not initialized — run agentledger init first"

	// run command.
	RunUse   = "run -- <cmd> [args]"
	RunShort = "Wrap a command as a recorded agent session."
	RunLong  = `Snapshot the Git working tree, run the command with its output
streamed live, snapshot again, and record the session, its file
changes, and its output. The child's exit code is propagated: if the
child exits non-zero, agentledger exits with the same code.`
	RunNoCommand      = "run requires a command: agentledger run -- <cmd> [args]"
	RunNotGitRepo     = "not a git repository: %s — run agentledger run inside a git repository"
	RunNotInitialized = "agentledger is not initialized in %s — run agentledger init first"
	RunSummary        = "Session %s finished: exit %d, %d file(s) changed"
	RunChildFailed    = "child exited with code %d"

	// Shared by log, show, export, undo.
	NotGitRepo     = "not a git repository: %s — run this command inside a git repository"
	NotInitialized = "agentledger is not initialized in %s — run agentledger init first"
	NoSessions     = "No sessions yet. Run: agentledger run -- <cmd>"
	SessionUnknown = "session %s not found — run agentledger log to list sessions"

	// log command.
	LogUse   = "log"
	LogShort = "List recorded sessions, newest first."
	LogLong  = `List recorded sessions, newest first. --limit N shows at most
N sessions; --limit 0 shows all.`
	LogLine = "%s %s %s %s exit=%s %d file(s)%s"

	// show command.
	ShowUse   = "show <id|last>"
	ShowShort = "Show detail for one session."
	ShowLong  = `Show detail for one session: fields, changed files, and captured
output. Use --diff to also print each file's diff. "last" means the
newest session.`
	ShowField      = "%s: %s"
	ShowFilesTitle = "Files changed:"
	ShowFileLine   = "  %s (%s)"
	ShowDiffTitle  = "Diff for %s (%s):"
	ShowOutputTitle = "Output:"
	ShowOutputLine = "  [%s] %s"
	ShowEventsHelp  = "show captured output events (output is shown by default)"
	ShowUndone     = " undone"

	// export command.
	ExportUse   = "export <id|last>"
	ExportShort = "Export one session as markdown or jsonl."
	ExportLong  = `Export one session to stdout: --format markdown for a readable
report, --format jsonl for one JSON object per line (session, then
actions, then events). "last" means the newest session.`
	ExportBadFormat = "invalid --format %q: use markdown or jsonl"

	// undo command.
	UndoUse   = "undo <id|last>"
	UndoShort = "Revert the working tree to before a session."
	UndoLong  = `Revert the working tree to the state before a session, using
its start snapshot. Modified and deleted files are checked out from
the start snapshot, added files are deleted, renamed files are moved
back. Never uses reset --hard: only the session's changed files are
affected. Refuses if those files changed again after the session
ended, unless --force is given. "last" means the newest session.`
	UndoFileHelp       = "only undo this file (repeatable)"
	UndoDryRunHelp     = "show what would be restored without changing anything"
	UndoForceHelp      = "undo even if the working tree is dirty (overwrites uncommitted changes)"
	UndoFileUnknown    = "file %s was not changed in session %s — run agentledger show %s to list changed files"
	UndoRenameUnknown  = "cannot undo rename for %s: original path not found in diff — undo aborted, nothing changed"
	UndoDirty          = "files changed since the session ended — undo would overwrite those later changes; revert them first, or retry with --force:\n%s"
	UndoDirtyWarning   = "Warning: files changed since the session ended — a real undo would refuse without --force"
	UndoNoChanges      = "Nothing to undo for session %s"
	UndoDryRunRestore  = "would restore %s (%s)"
	UndoDryRunDelete   = "would delete %s (added)"
	UndoDryRunSummary  = "Dry run: would restore %d file(s), no changes made"
	UndoSummary        = "Undone session %s: restored %d file(s)"
)
