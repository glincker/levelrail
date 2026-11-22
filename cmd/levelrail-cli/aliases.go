package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// runAliasedDeploy implements "deploy <app> ...", an alias for "apps deploy <app> ...".
func runAliasedDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "status" {
		return runAppsDeploysGate(prog, args[1:], stdout, stderr, lookupEnv)
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprintf(stderr, "alias for 'apps deploy' (image and commit deploys), see '%s apps deploy --help'\n\n", prog)
		_, _ = fmt.Fprint(stderr, deployUploadUsage(prog))
		return exitOK
	}
	if wantsRepoURLDeploy(args) {
		return runDeployURL(prog, args, stdout, stderr, lookupEnv)
	}
	if wantsUploadDeploy(args) {
		return runDeployUpload(prog, args, stdout, stderr, lookupEnv, os.Stdin)
	}
	_, _ = fmt.Fprintf(stderr, "alias for 'apps deploy', see '%s apps deploy --help'\n", prog)
	return runAppsDeploy(prog, args, stdout, stderr, lookupEnv, os.Stdin)
}

// runAliasedRollback implements "rollback <app> ...", an alias for "apps rollback <app> ...".
func runAliasedRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	_, _ = fmt.Fprintf(stderr, "alias for 'apps rollback', see '%s apps rollback --help'\n", prog)
	return runAppsRollback(prog, args, stdout, stderr, lookupEnv, os.Stdin)
}

// runAliasedLs implements "ls", an alias for "apps list".
func runAliasedLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runAppsList(prog, args, stdout, stderr, lookupEnv)
}

// runAliasedPs implements "ps", "apps list" narrowed to running apps unless the caller passes --status.
func runAliasedPs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	for _, a := range args {
		if a == "--status" || strings.HasPrefix(a, "--status=") || a == "-h" || a == "--help" {
			return runAppsList(prog, args, stdout, stderr, lookupEnv)
		}
	}
	return runAppsList(prog, append([]string{"--status", "running"}, args...), stdout, stderr, lookupEnv)
}

// runAliasedRm implements "rm <app>", an alias for "apps delete <app>" (it keeps the confirm prompt).
func runAliasedRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runAppsDelete(prog, args, stdout, stderr, lookupEnv)
}
