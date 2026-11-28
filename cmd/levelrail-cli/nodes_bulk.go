package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/glincker/levelrail/internal/apiclient"
)

// nodeSelector matches nodes by zone, region and provider; empty fields match all.
type nodeSelector struct{ zone, region, provider string }

func parseNodeSelector(where string) (nodeSelector, error) {
	var sel nodeSelector
	for _, part := range strings.Split(where, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || v == "" {
			return sel, newValidationError("--where %q must look like zone=eu-1,region=eu,provider=hetzner", where)
		}
		switch k {
		case "zone":
			sel.zone = v
		case "region":
			sel.region = v
		case "provider":
			sel.provider = v
		default:
			return sel, newValidationError("--where key %q must be zone, region or provider", k)
		}
	}
	return sel, nil
}

func (s nodeSelector) match(n nodeResource) bool {
	return (s.zone == "" || n.Zone == s.zone) && (s.region == "" || n.Region == s.region) && (s.provider == "" || n.Provider == s.provider)
}

// runNodesBulk implements "nodes bulk <cordon|uncordon|taint|set-labels> --where ...":
// it lists nodes once, selects client-side and applies the single-node call to each.
func runNodesBulk(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "nodes bulk", "print the per-node results as JSON to stdout and nothing else", stderr)
	where := fs.String("where", "", "select nodes: zone=Z, region=R, provider=P (comma separated, all must match)")
	dryRun := fs.Bool("dry-run", false, "list the matching nodes and change nothing")
	clearTaints := fs.Bool("clear", false, "taint: remove every taint")
	var setRegion, setProvider, setZone string
	fs.StringVar(&setRegion, "set-region", "", "set-labels: region label (empty clears it)")
	fs.StringVar(&setProvider, "set-provider", "", "set-labels: provider label (empty clears it)")
	fs.StringVar(&setZone, "set-zone", "", "set-labels: zone label (empty clears it)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, `Usage:
  %[1]s nodes bulk cordon|uncordon --where zone=Z [--dry-run]
  %[1]s nodes bulk taint --where zone=Z key[=value]:Effect ... | --clear
  %[1]s nodes bulk set-labels --where provider=P [--set-region R] [--set-provider P] [--set-zone Z]

Applies one action to every node matching --where, for example before
maintenance in a zone. Use --dry-run to see the matches first. Failures on one
node do not stop the others; the exit code is non-zero if any node failed.
`, prog)
	}
	if len(args) == 0 {
		fs.Usage()
		return exitUsage
	}
	action, rest := "", args
	if !strings.HasPrefix(args[0], "-") {
		action, rest = args[0], args[1:]
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if action != "cordon" && action != "uncordon" && action != "taint" && action != "set-labels" {
		return reportError(stdout, stderr, jsonOut, newValidationError("unknown bulk action %q (cordon, uncordon, taint, set-labels)", action))
	}
	if *where == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--where is required"))
	}
	sel, err := parseNodeSelector(*where)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	var taints []apiclient.NodeTaint
	var req setNodeFailureDomainReq
	switch action {
	case "taint":
		if taints, err = parseTaints(fs.Args()); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
		if len(taints) == 0 && !*clearTaints {
			return reportError(stdout, stderr, jsonOut, newValidationError("pass taints or --clear"))
		}
	case "set-labels":
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "set-region":
				req.Region = &setRegion
			case "set-provider":
				req.Provider = &setProvider
			case "set-zone":
				req.Zone = &setZone
			}
		})
		if req.Region == nil && req.Provider == nil && req.Zone == nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("pass at least one of --set-region, --set-provider, --set-zone"))
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	all, err := client.ListNodes(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list nodes: %w", err))
	}
	type result struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var results []result
	failed := 0
	for _, n := range all {
		if !sel.match(n) {
			continue
		}
		r := result{ID: n.ID, Name: n.Name, Status: "ok"}
		switch {
		case *dryRun:
			r.Status = "would " + action
		case action == "cordon":
			_, err = client.CordonNode(ctx, n.ID)
		case action == "uncordon":
			_, err = client.UncordonNode(ctx, n.ID)
		case action == "taint":
			_, err = client.SetNodeTaints(ctx, n.ID, taints)
		default:
			_, err = client.SetNodeFailureDomain(ctx, n.ID, req)
		}
		if err != nil {
			r.Status = "failed: " + err.Error()
			failed++
			err = nil
		}
		results = append(results, r)
	}
	if len(results) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("no nodes match --where %q", *where))
	}
	if err := renderResult(stdout, of.Format, of.Query, results, func() {
		for _, r := range results {
			_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", r.ID, r.Name, r.Status)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if failed > 0 {
		return exitCheckFailed
	}
	return exitOK
}
