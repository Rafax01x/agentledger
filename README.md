# agentledger

Git for AI agent actions.

Your AI agent changed 37 files. Do you remember why?

    agentledger run -- claude
    agentledger undo last

## Demo
<video src="https://github.com/Rafax01x/agentledger/raw/main/demo.mp4" controls width="100%"></video>

## Install

1. Download a prebuilt binary from the latest release at
   https://github.com/Rafax01x/agentledger/releases/latest,
   choose the binary for your operating system and
   architecture, make it executable, and put it on your PATH.
2. Or install with Go:
   `go install github.com/Rafax01x/agentledger/cmd/agentledger@latest`
3. Or build from source (requires Go >= 1.22 and a `git` binary):

```sh
git clone https://github.com/Rafax01x/agentledger.git
cd agentledger
go build -o agentledger ./cmd/agentledger
```

## Quickstart

In an existing Git repository, with at least one commit:

```sh
agentledger init
agentledger run -- sh -c "echo 'changed' >> a.txt && echo 'new' > c.txt"
agentledger log
agentledger show last --diff
agentledger undo last
```

The last step restores the working tree to its state before
the wrapped command ran.

## How it works

agentledger wraps a CLI agent command, snapshots the Git working tree before the command runs, lets the agent run with its output streaming live to your terminal, snapshots the tree again when the command finishes, records the session, its file changes, and its output in a local SQLite database, and lets you inspect or undo that session later with a single command.

```
user -> agentledger run -> snapshot -> agent -> snapshot -> SQLite -> undo
```

## Safety

- never touches your branch, commits, or index
- never sends data to any network
- never runs without an explicit `run` command
- `undo` refuses to run on dirty working tree unless --force

In practice, undo's dirty check compares each file against
the session's start and end snapshots, so a file already
restored to its start state does not block undo, and a
file changed after the session ended does.

## Limitations

See [docs/limitations.md](docs/limitations.md) for the honest
list of what v0.1 does not do.

## Roadmap

v0.2 candidates from [docs/limitations.md](docs/limitations.md):

- TOML config file support (default `--limit`, default export
  format, redaction on/off, agent aliases)
- `--agent` flag for `run` to override the agent name
  (currently the command basename)
- `log` filtering: by agent, by date range, by exit code,
  full-text search over commands and output
- `export --output <file>` instead of stdout-only export
- Session pruning / garbage collection of old
  `refs/agentledger/*` snapshots
- A pre-undo snapshot, so post-session edits overwritten
  with `--force` are preserved by agentledger
- `doctor --fix` to create a missing `.agentledger/`
  automatically
- Redaction for additional secret shapes (`github_pat_`,
  npm, PyPI, GCP keys, URLs with embedded passwords),
  and a `--no-redact` escape hatch for local debugging
- Undo for file-mode-only changes, and undo of an entire
  directory tree as one unit
- A `run --dry-run` that snapshots without running, for
  testing snapshot behaviour
- Shell completions and a man page

## Contributing

Contributions are welcome. See
[CONTRIBUTING.md](CONTRIBUTING.md) for how to build, test,
and submit a pull request.
