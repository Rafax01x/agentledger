// Package ui serves a local, single-page web inspector for
// agentledger sessions. It uses only the Go standard library:
// net/http plus an embedded static page. The server binds to
// loopback only and never sends data anywhere: it is a local view
// over the repository's own SQLite database.
//
// Undo through the UI reuses the CLI's undo logic (passed in as
// Undo), so the web button enforces exactly the same safety rules
// as `agentledger undo`, including the post-session-change refusal
// signalled by ErrDirty.
package ui

import (
	"embed"
	"errors"
)

//go:embed static/index.html
var staticFS embed.FS

// ErrDirty signals that the working tree changed after the session
// ended, so an undo without force was refused.
var ErrDirty = errors.New("working tree changed since session ended")

// UndoFunc reverts a session and returns the number of files
// restored. An empty files list means a full undo; a non-empty
// list reverts only those files (the per-file Revert buttons).
// It is provided by the CLI layer, which owns the undo
// implementation; the UI layer only calls it.
type UndoFunc func(sessionID string, force bool, files []string) (int, error)
