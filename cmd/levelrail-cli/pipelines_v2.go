package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runPipelinesArtifacts lists the artifacts a run stored, or downloads one
// job's archive with --download.
func runPipelinesArtifacts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines artifacts", "print the artifacts as JSON", stdout, stderr)
	job := c.fs.String("download", "", "job key whose artifact archive to download")
	out := c.fs.String("out", "", "file to write the download to (default: run-<id>-<job>.tar.gz in the current directory)")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	ctx := context.Background()
	list, err := client.ListPipelineRunArtifacts(ctx, pos[0], pos[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list artifacts of run %q: %w", pos[1], err))
	}
	if *job == "" {
		return c.write(of, list, func() { printArtifacts(stdout, list) })
	}
	for _, a := range list {
		if a.Job != *job {
			continue
		}
		return downloadArtifact(ctx, client, pos[0], pos[1], a, *out, of, c, stdout, stderr, jsonOut)
	}
	return reportError(stdout, stderr, jsonOut, fmt.Errorf("run %q has no artifact from job %q", pos[1], *job))
}

func downloadArtifact(ctx context.Context, client *Client, app, runID string, a apiclient.PipelineArtifact, out string, of outputFlags, c *pipelineCmd, stdout, stderr io.Writer, jsonOut bool) int {
	data, err := client.DownloadPipelineArtifact(ctx, app, runID, a.ID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download artifact of job %q: %w", a.Job, err))
	}
	if out == "" {
		out = fmt.Sprintf("run-%s-%s.tar.gz", runID, filepath.Base(a.Job))
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", out, err))
	}
	return c.write(of, map[string]any{"file": out, "bytes": len(data), "sha256": a.SHA256}, func() {
		_, _ = fmt.Fprintf(stdout, "wrote %s (%d bytes, sha256 %s)\n", out, len(data), a.SHA256)
	})
}

func printArtifacts(out io.Writer, list []apiclient.PipelineArtifact) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "this run stored no artifacts")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "JOB\tSIZE\tSHA256\tEXPIRES")
	for _, a := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", a.Job, a.SizeBytes, a.SHA256, a.ExpiresAt.Local().Format(time.DateTime))
	}
	_ = tw.Flush()
}

// runPipelinesWebhook manages a pipeline's inbound webhook trigger:
// `pipelines triggers <app> <pipeline> [--enable|--disable|--rotate|--delete|--deliveries|--replay ID]`.
func runPipelinesWebhook(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines triggers", "print the result as JSON", stdout, stderr)
	enable := c.fs.Bool("enable", false, "create the webhook trigger, or turn it back on")
	disable := c.fs.Bool("disable", false, "turn the webhook trigger off without deleting it")
	hmacMode := c.fs.Bool("hmac", false, "require a GitHub-style sha256 signature (with --enable)")
	rotate := c.fs.Bool("rotate", false, "mint a new URL token (and signing secret), invalidating the old ones")
	del := c.fs.Bool("delete", false, "delete the webhook trigger")
	deliveries := c.fs.Bool("deliveries", false, "list recent deliveries")
	replay := c.fs.String("replay", "", "delivery ID to replay as a new run")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	ctx, app, name := context.Background(), pos[0], pos[1]
	fail := func(what string, err error) int {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for pipeline %q: %w", what, name, err))
	}
	switch {
	case *deliveries:
		list, err := client.ListPipelineWebhookDeliveries(ctx, app, name)
		if err != nil {
			return fail("list webhook deliveries", err)
		}
		return c.write(of, list, func() { printDeliveries(stdout, list) })
	case *replay != "":
		run, err := client.ReplayPipelineWebhookDelivery(ctx, app, name, *replay)
		if err != nil {
			return fail("replay webhook delivery", err)
		}
		return c.write(of, run, func() { _, _ = fmt.Fprintf(stdout, "run #%d started (%s)\n", run.Number, run.ID) })
	case *del:
		if err := client.DeletePipelineWebhook(ctx, app, name); err != nil {
			return fail("delete webhook trigger", err)
		}
		return c.write(of, map[string]string{"deleted": name}, func() { _, _ = fmt.Fprintln(stdout, "webhook trigger deleted") })
	case *rotate:
		wh, err := client.RotatePipelineWebhook(ctx, app, name)
		if err != nil {
			return fail("rotate webhook trigger", err)
		}
		return c.write(of, wh, func() { printWebhook(stdout, wh) })
	case *enable || *disable || c.fs.Lookup("hmac").Value.String() == "true":
		req := apiclient.PipelineWebhookRequest{}
		if *enable || *disable {
			on := *enable && !*disable
			req.Enabled = &on
		}
		if *hmacMode {
			req.HMAC = hmacMode
		}
		wh, err := client.SetPipelineWebhook(ctx, app, name, req)
		if err != nil {
			return fail("update webhook trigger", err)
		}
		return c.write(of, wh, func() { printWebhook(stdout, wh) })
	}
	wh, err := client.GetPipelineWebhook(ctx, app, name)
	if err != nil {
		return fail("read webhook trigger", err)
	}
	return c.write(of, wh, func() { printWebhook(stdout, wh) })
}

func printWebhook(out io.Writer, wh apiclient.PipelineWebhook) {
	if !wh.Configured {
		_, _ = fmt.Fprintln(out, "no webhook trigger. Create one with --enable (add --hmac to require signatures).")
		return
	}
	state := "enabled"
	if !wh.Enabled {
		state = "disabled"
	}
	mode := "token only"
	if wh.HMAC {
		mode = "token + sha256 signature (X-Hub-Signature-256)"
	}
	_, _ = fmt.Fprintf(out, "webhook %s, %s\nPOST %s\n", state, mode, wh.Path)
	if wh.Token != "" {
		_, _ = fmt.Fprintln(out, "The token is shown once. Store it now; it cannot be read back.")
	}
	if wh.Secret != "" {
		_, _ = fmt.Fprintf(out, "Signing secret (shown once): %s\n", wh.Secret)
	}
}

func printDeliveries(out io.Writer, list []apiclient.PipelineWebhookDelivery) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no deliveries yet")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTIME\tOUTCOME\tSTATUS\tRUN\tREASON")
	for _, d := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", d.ID, d.ReceivedAt.Local().Format(time.DateTime), d.Outcome, d.StatusCode, d.RunID, d.Reason)
	}
	_ = tw.Flush()
}
