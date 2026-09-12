package main

import (
	"context"
	"fmt"
	"io"
)

// runSystemPrune implements "system-prune": POST /api/v1/system/prune
// (internal/api/system_prune.go), AbilityRoot-gated. Removes every
// stopped container, dangling image, and unused volume or build cache
// the reconciler's current desired state doesn't need, fleet-wide.
func runSystemPrune(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system-prune", "print the prune result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, systemPruneUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.PruneSystem(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("prune system: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printSystemPruneResultHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSystemPruneResultHuman(out io.Writer, r systemPruneResult) {
	_, _ = fmt.Fprintf(out, "containers removed:    %d (%d bytes reclaimed)\n", len(r.ContainersRemoved), r.ContainersReclaimedBytes)
	_, _ = fmt.Fprintf(out, "images removed:        %d (%d bytes reclaimed)\n", len(r.ImagesRemoved), r.ImagesReclaimedBytes)
	_, _ = fmt.Fprintf(out, "volumes removed:       %d (%d bytes reclaimed)\n", len(r.VolumesRemoved), r.VolumesReclaimedBytes)
	_, _ = fmt.Fprintf(out, "build cache reclaimed: %d bytes\n", r.BuildCacheReclaimedBytes)
	for _, e := range r.Errors {
		_, _ = fmt.Fprintf(out, "error: %s\n", e)
	}
}

func systemPruneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s system-prune [flags]

Removes every stopped container, dangling image, and unused volume or
build cache not part of the reconciler's current desired state,
fleet-wide. Requires an admin/root-scoped token, the same as
"%[1]s audit-purge".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the prune result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
