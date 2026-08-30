package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsDelete implements "apps delete <name>": DELETE
// /api/v1/apps/{name} (internal/api/apps.go's own handleDeleteApp doc
// comment covers the known gap that this removes desired state only, it
// does not itself stop the running container).
func runAppsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	name, ok := requireOneArg(fs, stderr, prog, "apps delete", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	if err := client.DeleteApp(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "app %q deleted\n", name)
	return exitOK
}

func appsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps delete <name> [flags]

Removes an app's desired state. Does not itself stop the running
container; the next reconcile pass tears it down.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
