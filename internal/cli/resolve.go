package cli

import (
	"fmt"
	"os"

	"agentledger/internal/gitx"
	"agentledger/internal/store"
)

// openStoreForRepo finds the repo root from the working directory,
// checks that agentledger is initialized there, and opens the database.
// The caller must close the returned store.
func openStoreForRepo() (string, *store.Store, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("determine working directory: %w", err)
	}
	root, err := gitx.FindRoot(cwd)
	if err != nil {
		return "", nil, fmt.Errorf(NotGitRepo, cwd)
	}
	db := dbPath(root)
	if _, err := os.Stat(db); err != nil {
		return "", nil, fmt.Errorf(NotInitialized, agentledgerDir(root))
	}
	st, err := store.Open(db)
	if err != nil {
		return "", nil, err
	}
	return root, st, nil
}

// resolveSession turns an id argument into a session. "last" means
// the newest session; anything else is looked up by id.
func resolveSession(st *store.Store, idOrLast string) (*store.Session, error) {
	if idOrLast == "last" {
		sessions, err := st.ListSessions(1)
		if err != nil {
			return nil, err
		}
		if len(sessions) == 0 {
			return nil, fmt.Errorf("%s", NoSessions)
		}
		return sessions[0], nil
	}
	sess, err := st.GetSession(idOrLast)
	if err != nil {
		return nil, fmt.Errorf(SessionUnknown, idOrLast)
	}
	return sess, nil
}
