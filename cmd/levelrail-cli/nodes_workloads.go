package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesWorkloads implements "nodes workloads <id> --accepts-app
// --accepts-build": PUT /api/v1/nodes/{id}/workloads. The API takes a
// full replace of both flags, not a per-field patch
// (setNodeWorkloadsRequest's own doc comment), so both flags are
// required here rather than defaulting an omitted one to false: an
// operator who means "leave build workloads as they are" would
// otherwise silently disable them. fs.Visit after Parse is what tells
// "flag omitted" apart from "flag explicitly set to false", a
// distinction plain BoolVar defaults alone can't make.
func runNodesWorkloads(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "nodes workloads", "print the updated node as JSON to stdout and nothing else", stderr)
	var acceptsApp, acceptsBuild bool
	fs.BoolVar(&acceptsApp, "accepts-app", false, "whether this node accepts app workloads (required)")
	fs.BoolVar(&acceptsBuild, "accepts-build", false, "whether this node accepts build workloads (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesWorkloadsUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "nodes workloads", "node id")
	if !ok {
		return exitUsage
	}

	var appSet, buildSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "accepts-app":
			appSet = true
		case "accepts-build":
			buildSet = true
		}
	})
	if !appSet {
		return reportError(stdout, stderr, jsonOut, newValidationError("--accepts-app is required"))
	}
	if !buildSet {
		return reportError(stdout, stderr, jsonOut, newValidationError("--accepts-build is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	updated, err := client.SetNodeWorkloads(context.Background(), id, setNodeWorkloadsRequest{
		AcceptsAppWorkloads:   acceptsApp,
		AcceptsBuildWorkloads: acceptsBuild,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set workloads for node %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "node %q workloads set: accepts_app_workloads=%t accepts_build_workloads=%t\n", id, updated.AcceptsAppWorkloads, updated.AcceptsBuildWorkloads)
	return exitOK
}

func nodesWorkloadsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]

Sets a node's accepted workload kinds. This is a full replace, not a
per-field patch: both flags are required, and whatever value each one
currently holds is overwritten with what's passed here.

Flags:
  --accepts-app bool       whether this node accepts app workloads (required)
  --accepts-build bool     whether this node accepts build workloads (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the updated node as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
