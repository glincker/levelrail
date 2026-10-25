package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// scheduleDeployAt creates a one-shot scheduled deploy instead of deploying now.
// Exactly one of image or ref is set; at is an RFC 3339 time or a duration like "in 2h".
func scheduleDeployAt(prog string, client *Client, name, at, image, ref, idemKey string, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	when, err := parseRunAt(at, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("--at: %v", err))
	}
	ctx := apiclient.WithIdempotencyKey(context.Background(), idemKey)
	sd, err := client.CreateScheduledDeploy(ctx, name, apiclient.ScheduledDeployRequest{
		RunAt: when.UTC().Format(time.RFC3339), Image: image, Ref: ref,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("schedule deploy for %s: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, sd, func() {
		_, _ = fmt.Fprintf(stdout, "scheduled %s for %s at %s; cancel with \"%s apps schedule cancel %s %s\"\n",
			describeScheduledDeploy(sd), name, sd.RunAt.Local().Format("2006-01-02 15:04:05 MST"), prog, name, sd.ID)
	})
}

func describeScheduledDeploy(sd apiclient.ScheduledDeploy) string {
	if sd.Kind == "image" {
		return "image " + sd.Image
	}
	return "git ref " + sd.Ref
}

func runAppsScheduleList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps schedule list", "print the scheduled deploys as JSON to stdout and nothing else", stderr)
	var status string
	fs.StringVar(&status, "status", "", "only these statuses, comma separated (pending, fired, missed, skipped_freeze, canceled, failed, pending_approval)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps schedule list <app> [flags]\n\nLists <app>'s one-shot scheduled deploys.\n\nFlags:\n  --status string  only these statuses, comma separated\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps schedule list", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rows, err := client.ListScheduledDeploys(context.Background(), rest[0], status)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list scheduled deploys for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintf(stdout, "no scheduled deploys for %s\n", rest[0])
			return
		}
		for _, r := range rows {
			line := fmt.Sprintf("%s  %-16s  %s  %s", r.ID, r.Status, r.RunAt.Local().Format("2006-01-02 15:04:05"), describeScheduledDeploy(r))
			if r.Reason != "" {
				line += "  (" + r.Reason + ")"
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	})
}

func runAppsScheduleCancel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps schedule cancel", "print the canceled scheduled deploy as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps schedule cancel <app> <id> [flags]\n\nCancels a pending one-shot scheduled deploy.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps schedule cancel", "an app name and a scheduled deploy id", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	sd, err := client.CancelScheduledDeploy(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cancel scheduled deploy %s: %w", rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, sd, func() {
		_, _ = fmt.Fprintf(stdout, "canceled scheduled deploy %s for %s\n", sd.ID, rest[0])
	})
}
