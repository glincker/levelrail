package main

import (
	"context"
	"fmt"
	"io"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/glincker/levelrail/internal/resourceadvisor"
)

// advisorKinds maps the --kind flag to the finding kind it selects.
var advisorKinds = map[string]resourceadvisor.Kind{"idle": resourceadvisor.KindIdleApp}

type advisorBulkEntry struct {
	FindingID string `json:"finding_id"`
	Resource  string `json:"resource"`
	Applied   bool   `json:"applied"`
	Error     string `json:"error,omitempty"`
}

// runAdvisorApplyAll previews or applies every appliable finding of one kind.
func runAdvisorApplyAll(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "advisor apply", "print the result as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "apply the changes; without it only the list is shown")
	fs.Bool("all", false, "apply every matching finding")
	kind := fs.String("kind", "", "which findings to apply: idle")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	want, known := advisorKinds[*kind]
	if !known {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("--all needs --kind idle"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListAdvisorFindings(context.Background(), apiclient.AdvisorFindingQuery{})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list advisor findings: %w", err))
	}
	var out []advisorBulkEntry
	failed := false
	for _, f := range list.Findings {
		if f.Kind != want || !f.Appliable || !f.CanApply {
			continue
		}
		e := advisorBulkEntry{FindingID: f.ID, Resource: f.Resource.String()}
		res, err := client.ApplyAdvisorFinding(context.Background(), f.ID, *yes)
		if err != nil {
			e.Error, failed = err.Error(), true
		} else {
			e.Applied = res.Applied
		}
		out = append(out, e)
	}
	code := writeScheduledTaskResult(stdout, stderr, of, out, func() {
		for _, e := range out {
			switch {
			case e.Error != "":
				_, _ = fmt.Fprintf(stdout, "%s: failed: %s\n", e.Resource, e.Error)
			case e.Applied:
				_, _ = fmt.Fprintf(stdout, "%s: sleep when idle enabled\n", e.Resource)
			default:
				_, _ = fmt.Fprintf(stdout, "%s: would enable sleep when idle\n", e.Resource)
			}
		}
		switch {
		case len(out) == 0:
			_, _ = fmt.Fprintln(stdout, "no idle findings to apply")
		case !*yes:
			_, _ = fmt.Fprintln(stdout, "preview only, nothing changed. Run again with --yes to apply.")
		}
	})
	if failed && code == exitOK {
		return exitUsage
	}
	return code
}
