package main

import (
	"errors"
	"os"

	"agentledger/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// Propagate a wrapped child's exit code instead of the generic 1.
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}
