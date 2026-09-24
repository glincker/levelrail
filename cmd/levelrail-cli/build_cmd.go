package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runBuild dispatches "build <verb> [flags]": pre-flight checks against a
// repository URL before any app exists.
func runBuild(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, buildUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, buildUsage(prog))
		return exitOK
	case "detect":
		return runBuildRepoCommand(prog, "detect", args[1:], stdout, stderr, lookupEnv)
	case "branches":
		return runBuildRepoCommand(prog, "branches", args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown build subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, buildUsage(prog))
		return exitUsage
	}
}

func buildUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s build detect --repo-url URL [--ref REF] [flags]   show what framework the builder detects for a public repo, no build runs
  %[1]s build branches --repo-url URL [flags]             list the branches a public repo advertises

Both work against a public repository and need no existing app.

Run "%[1]s build <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func buildRepoUsage(prog, verb string) string {
	refLine := ""
	if verb == "detect" {
		refLine = "  --ref string              branch, tag or commit to inspect (default: the repo's default branch)\n"
	}
	return fmt.Sprintf(`Usage:
  %[1]s build %[5]s --repo-url URL [flags]

Flags:
  --repo-url string         public repository URL (required)
%[6]s  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, verb, refLine)
}

func runBuildRepoCommand(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "build "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var repoURL, ref string
	fs.StringVar(&repoURL, "repo-url", "", "public repository URL (required)")
	if verb == "detect" {
		fs.StringVar(&ref, "ref", "", "branch, tag or commit to inspect")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, buildRepoUsage(prog, verb)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: build %s takes no positional arguments\n\n", prog, verb)
		fs.Usage()
		return exitUsage
	}
	if strings.TrimSpace(repoURL) == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--repo-url is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if verb == "branches" {
		res, err := client.ListRepoBranches(ctx, repoURL)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list branches for %q: %w", repoURL, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			for _, b := range res.Branches {
				_, _ = fmt.Fprintln(stdout, b)
			}
			if len(res.Branches) == 0 {
				_, _ = fmt.Fprintln(stdout, "no branches")
			}
		})
	}

	res, err := client.DetectFramework(ctx, repoURL, ref)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("detect framework for %q: %w", repoURL, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if !res.Detected {
			_, _ = fmt.Fprintln(stdout, "no framework detected (pick a build type manually)")
			return
		}
		_, _ = fmt.Fprintf(stdout, "detected: %s (provider: %s)\n", res.FrameworkName, dashIfEmpty(res.Provider))
	})
}
