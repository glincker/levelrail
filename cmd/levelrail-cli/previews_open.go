package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// previewOpenURL turns a preview's domain into a browsable URL, or "" when
// the preview has none yet (still building).
func previewOpenURL(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	if strings.Contains(domain, "://") {
		return domain
	}
	return "https://" + domain
}

func appsPreviewsOpenUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews open <app-name> <pr-number> [--print] [flags]

Opens the preview's URL in your browser. With --print (or when no browser can
be launched) the URL is written to stdout instead.
`, prog)
}

func runAppsPreviewsOpen(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps previews open", "print the preview URL as JSON", stderr)
	printOnly := fs.Bool("print", false, "print the URL instead of opening a browser")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsOpenUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	positional := fs.Args()
	if len(positional) != 2 {
		_, _ = fmt.Fprint(stderr, appsPreviewsOpenUsage(prog))
		return exitUsage
	}
	prNumber, err := strconv.Atoi(positional[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("pr number must be an integer"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	previews, err := client.ListPreviewEnvironments(context.Background(), positional[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list previews for app %q: %w", positional[0], err))
	}
	for _, p := range previews {
		if p.PRNumber != prNumber {
			continue
		}
		url := previewOpenURL(p.Domain)
		if url == "" {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview for app %q pr #%d has no URL yet (status %s)", positional[0], prNumber, p.Status))
		}
		if jsonOut {
			_, _ = fmt.Fprintf(stdout, "{\"url\":%q}\n", url)
			return exitOK
		}
		if *printOnly || openBrowser(context.Background(), url) != nil {
			_, _ = fmt.Fprintln(stdout, url)
		}
		return exitOK
	}
	return reportError(stdout, stderr, jsonOut, fmt.Errorf("no preview for app %q pr #%d", positional[0], prNumber))
}
