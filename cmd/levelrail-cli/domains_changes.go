package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

func runDomainsChanges(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains changes", "print the changes as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains changes <domain> [--json]\n\nLists recent changes to the domain with the time until each can be rolled back.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains changes", "a domain", 1)
	if !ok {
		return exitUsage
	}
	domain := strings.ToLower(rest[0])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	changes, err := client.DomainChanges(context.Background(), domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list changes for %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, changes, func() {
		if len(changes) == 0 {
			_, _ = fmt.Fprintln(stdout, "no recorded changes for "+domain)
		}
		for _, c := range changes {
			state := "rollback until " + c.RollbackUntil.Format(time.RFC3339)
			switch {
			case c.AlreadyUndone:
				state = "rolled back"
			case !c.CanRollback:
				state = "cannot be rolled back"
			}
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s  %s\n", c.RunID, c.CreatedAt.Format(time.RFC3339), c.Kind, state)
			for _, e := range c.Summary {
				_, _ = fmt.Fprintf(stdout, "    %s: %s\n", e.Action, e.Description)
			}
		}
	})
}

func runDomainsRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains rollback", "print the result as JSON to stdout and nothing else", stderr)
	var yes bool
	fs.BoolVar(&yes, "yes", false, "roll back now; without it only what would change is printed")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains rollback <domain> <run-id> [--yes] [--json]\n\nRemoves the records the change created and restores the ones it replaced. Records edited since are left alone.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains rollback", "a domain and a run id", 2)
	if !ok {
		return exitUsage
	}
	domain, id := strings.ToLower(rest[0]), rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if !yes {
		changes, err := client.DomainChanges(ctx, domain)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list changes for %q: %w", domain, err))
		}
		for _, c := range changes {
			if c.RunID != id {
				continue
			}
			if !c.CanRollback {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("change %q can no longer be rolled back", id))
			}
			_, _ = fmt.Fprintf(stdout, "rolling back %s would undo (until %s):\n", id, c.RollbackUntil.Format(time.RFC3339))
			for _, e := range c.Summary {
				_, _ = fmt.Fprintf(stdout, "  %s: %s\n", e.Action, e.Description)
			}
			_, _ = fmt.Fprintln(stdout, "pass --yes to roll back")
			return exitOK
		}
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("change %q not found for %q", id, domain))
	}
	res, err := client.RollbackDomainChange(ctx, domain, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		for _, o := range res.Outcomes {
			line := o.Status + ": " + o.Label
			if o.Message != "" {
				line += " (" + o.Message + ")"
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	})
}
