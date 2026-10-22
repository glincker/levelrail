package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envProvisionPoll    = "APP_CLI_PROVISION_POLL_INTERVAL"
	envProvisionTimeout = "APP_CLI_PROVISION_WAIT_TIMEOUT"
)

func envDurationOr(lookupEnv func(string) (string, bool), key string, def time.Duration) time.Duration {
	if v, ok := lookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

func provisionMark(status string) string {
	switch status {
	case "done":
		return "[x]"
	case "started":
		return "[>]"
	case "failed":
		return "[!]"
	case "skipped":
		return "[-]"
	}
	return "[ ]"
}

func printProvisionStep(w io.Writer, s apiclient.ProvisionStepResource) {
	line := fmt.Sprintf("%s %s", provisionMark(s.Status), s.Step)
	if s.Status == "started" && s.Total > 0 && s.Step == "pull_image" {
		line += fmt.Sprintf(" %d%%", s.Current*100/s.Total)
	}
	if s.Detail != "" && s.Status != "pending" {
		line += ": " + s.Detail
	}
	_, _ = fmt.Fprintln(w, line)
}

func printProvisionAttempt(w io.Writer, a apiclient.ProvisionAttemptResource) {
	_, _ = fmt.Fprintf(w, "provisioning %s (%s)\n", a.ID, a.Status)
	for _, s := range a.Steps {
		printProvisionStep(w, s)
	}
	if a.Failure != nil {
		_, _ = fmt.Fprintf(w, "failed at %s: %s\n", a.Failure.Step, a.Failure.Reason)
		if a.Failure.Fix != nil {
			_, _ = fmt.Fprintf(w, "fix: %s\n", a.Failure.Fix.Label)
		}
	}
}

// waitProvision prints each step change until the attempt is ready or failed.
func waitProvision(ctx context.Context, client *Client, name string, w io.Writer, lookupEnv func(string) (string, bool)) int {
	poll := envDurationOr(lookupEnv, envProvisionPoll, time.Second)
	ctx, cancel := context.WithTimeout(ctx, envDurationOr(lookupEnv, envProvisionTimeout, 15*time.Minute))
	defer cancel()
	seen := map[string]string{}
	for {
		a, err := client.GetDatabaseProvision(ctx, name)
		if err == nil {
			for _, s := range a.Steps {
				key := s.Status + "|" + s.Detail
				if s.Status != "pending" && seen[s.Step] != key {
					seen[s.Step] = key
					printProvisionStep(w, s)
				}
			}
			switch a.Status {
			case "ready":
				return exitOK
			case "failed":
				if a.Failure != nil {
					_, _ = fmt.Fprintf(w, "failed at %s: %s\n", a.Failure.Step, a.Failure.Reason)
				}
				return exitValidation
			}
		}
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintln(w, "timed out waiting for the database to become ready")
			return exitValidation
		case <-time.After(poll):
		}
	}
}
