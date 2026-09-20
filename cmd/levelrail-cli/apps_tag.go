package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsTag implements "apps tag <name> <tag>": POST
// /api/v1/apps/{name}/tags. tag is a name, not an ID, creating it first
// if this is the first time it's been used (see internal/api's
// attachAppTagRequest own doc comment).
func runAppsTag(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps tag", "print the attached tag as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsTagUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps tag", "exactly two arguments: app name and tag", 2)
	if !ok {
		return exitUsage
	}
	appName, tagName := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	attached, err := client.AttachAppTag(context.Background(), appName, tagName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("tag app %q with %q: %w", appName, tagName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, attached, func() {
		_, _ = fmt.Fprintf(stdout, "app %q tagged %q\n", appName, attached.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsTagUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps tag <name> <tag> [flags]

Attaches a tag (by name) to an app, creating the tag first if it
doesn't exist yet.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the attached tag as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsUntag implements "apps untag <name> <tag-id>": DELETE
// /api/v1/apps/{name}/tags/{id}. tagID is the tag's ID (from "tags
// list" or "apps get <name>"'s own tags field), not its name.
func runAppsUntag(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps untag", "print {\"detached\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsUntagUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps untag", "exactly two arguments: app name and tag id", 2)
	if !ok {
		return exitUsage
	}
	appName, tagID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DetachAppTag(context.Background(), appName, tagID); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("untag app %q of %q: %w", appName, tagID, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"detached": true}, func() {
		_, _ = fmt.Fprintf(stdout, "app %q untagged (tag id %s)\n", appName, tagID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsUntagUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps untag <name> <tag-id> [flags]

Detaches a tag (by ID) from an app. The tag itself still exists and
stays attached to any other app it's on.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"detached": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
