package main

import (
	"fmt"
	"io"
	"os"
)

// runAliasedDeploy implements "deploy <app> ...", an alias for "apps deploy <app> ...".
func runAliasedDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	_, _ = fmt.Fprintf(stderr, "alias for 'apps deploy', see '%s apps deploy --help'\n", prog)
	return runAppsDeploy(prog, args, stdout, stderr, lookupEnv, os.Stdin)
}

// runAliasedRollback implements "rollback <app> ...", an alias for "apps rollback <app> ...".
func runAliasedRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	_, _ = fmt.Fprintf(stderr, "alias for 'apps rollback', see '%s apps rollback --help'\n", prog)
	return runAppsRollback(prog, args, stdout, stderr, lookupEnv, os.Stdin)
}
