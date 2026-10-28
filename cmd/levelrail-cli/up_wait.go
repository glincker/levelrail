package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// waitUpDeploy follows the deploy up just started and records its verdict in result.
func waitUpDeploy(ctx context.Context, client *Client, result *upResult, timeout time.Duration, stderr io.Writer, jsonOut bool) error {
	name := result.App
	var logDone chan struct{}
	stopLog := func() {}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "waiting for %q to become healthy\n", name)
		followCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		logDone = make(chan struct{})
		stopLog = cancel
		deployID := result.DeployID
		if deployID == "" {
			deployID = "latest"
		}
		go func() {
			defer close(logDone)
			_ = client.StreamDeployLog(followCtx, name, deployID, func(e apiclient.LogStreamEntry) error {
				_, _ = fmt.Fprintln(stderr, e.Line)
				return nil
			})
		}()
	}
	wait, werr := client.WaitForDeploy(ctx, name, result.DeployID, apiclient.WaitOptions{Interval: 2 * time.Second, Window: timeout})
	if logDone != nil {
		select {
		case <-logDone:
		case <-time.After(logDrainGrace):
		}
		stopLog()
		<-logDone
	}
	if werr != nil && !errors.Is(werr, context.Canceled) {
		return werr
	}
	result.DeployID = wait.Deploy.ID
	result.Status = wait.Deploy.Outcome
	if !wait.Done {
		result.Status = "timeout"
	}
	result.Failure, result.Error = wait.Deploy.Failure, wait.Deploy.Error
	if final, err := client.GetApp(ctx, name); err == nil {
		result.URL = appPrimaryURL(final)
		if domain := primaryDomain(final); domain != "" && result.Status == apiclient.DeployOutcomeHealthy {
			result.Certificate = domainCertState(ctx, client, domain)
		}
	}
	return nil
}

// upWaitExit maps the waited deploy's verdict to the same exit codes "deploy" uses.
func upWaitExit(status string) int {
	switch status {
	case apiclient.DeployOutcomeHealthy:
		return exitOK
	case "timeout":
		return exitDeployTimeout
	default:
		return exitDeployNotHealthy
	}
}

func printUpWaitVerdict(stderr io.Writer, result *upResult) {
	_, _ = fmt.Fprintf(stderr, "deploy %s: %s\n", result.DeployID, result.Status)
	if result.Status == apiclient.DeployOutcomeHealthy {
		if note := certNote(result.URL, result.Certificate); note != "" {
			_, _ = fmt.Fprintln(stderr, note)
		}
		return
	}
	printUploadFailure(stderr, &deployUploadResult{Failure: result.Failure, Error: result.Error})
}
