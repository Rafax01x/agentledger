package cli

import "path/filepath"

// agentledgerDir returns the .agentledger/ directory for a repo root.
func agentledgerDir(root string) string {
	return filepath.Join(root, ".agentledger")
}

// dbPath returns the SQLite database file for a repo root.
func dbPath(root string) string {
	return filepath.Join(agentledgerDir(root), "agentledger.db")
}
