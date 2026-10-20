package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

var repoURLArg = regexp.MustCompile(`^(?:(?:https?|ssh|git)://\S+|[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:\S+)$`)

// wantsRepoURLDeploy reports whether "deploy" args carry a git URL, which a directory path or app name never matches.
func wantsRepoURLDeploy(args []string) bool {
	for i, a := range args {
		if strings.HasPrefix(a, "-") || !repoURLArg.MatchString(a) {
			continue
		}
		if i > 0 && isURLValuedFlag(args[i-1]) {
			continue
		}
		return true
	}
	return false
}

// isURLValuedFlag reports whether a is a flag whose next argument is the control plane URL, not a repository.
func isURLValuedFlag(a string) bool {
	name := strings.TrimLeft(a, "-")
	return strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && (name == "url" || name == "api-url" || name == "api")
}

// runDeployURL implements "deploy <git-url>": analyze the repository, then create the app and start its first build.
func runDeployURL(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "deploy", "print the result as JSON to stdout and nothing else", stderr)
	f := importFlags{env: map[string]string{}}
	var af importAnalyzeFlags
	var dryRun bool
	fs.StringVar(&f.name, "name", "", "app name (default: the repository name)")
	fs.StringVar(&f.name, "app", "", "same as --name")
	fs.StringVar(&f.ref, "branch", "", "branch to build (default: main, then master)")
	fs.IntVar(&f.port, "port", 0, "container port override")
	fs.Var(stringMapFlag(f.env), "env", "env value as KEY=VALUE, repeatable (satisfies required variables)")
	fs.StringVar(&af.baseDir, "base-directory", "", "monorepo app directory override")
	fs.StringVar(&af.dockerfile, "dockerfile", "", "Dockerfile path override, relative to the repository root")
	fs.StringVar(&af.buildType, "build-type", "", "build type override: dockerfile, railpack or static")
	fs.BoolVar(&af.noDatabase, "no-database", false, "do not create or attach the suggested databases")
	fs.BoolVar(&dryRun, "dry-run", false, "print what would be deployed and create nothing")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deployURLUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("deploy takes exactly one git URL"))
	}
	af.analyze, af.apply = dryRun, !dryRun
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	return runImportAnalyze(client, f, af, rest, stdout, stderr, jsonOut, of)
}

func deployURLUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy <repo-url> [flags]

Reads the repository (GitHub, GitLab, Gitea, Bitbucket; public, or private through a
connected provider app), infers framework, branch, name and port, creates the app and
starts the first build.

Flags:
  --name NAME, --app NAME  app name (default: the repository name)
  --branch BRANCH          branch to build (default: main, then master)
  --port N                 container port override
  --env KEY=VALUE          env value, repeatable (satisfies required variables)
  --base-directory DIR     monorepo app directory override
  --dockerfile PATH        Dockerfile path override
  --build-type TYPE        dockerfile, railpack or static
  --no-database            do not create or attach the suggested databases
  --dry-run                print what would be deployed and create nothing
`, prog)
}
