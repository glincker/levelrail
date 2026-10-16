package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type gitSourceWebhookInfo struct {
	URL         string   `json:"url"`
	ContentType string   `json:"content_type"`
	Events      []string `json:"events"`
	Secret      string   `json:"secret,omitempty"`
}

// absoluteWebhookURL joins a path-only webhook_url onto the API origin so
// the printed value can be pasted into a git provider as is.
func absoluteWebhookURL(base, raw string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(raw, "/")
}

func runAppsGitSourceWebhook(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source webhook", "print the webhook settings as JSON to stdout and nothing else", stderr)
	var rotate bool
	fs.BoolVar(&rotate, "rotate", false, "mint a fresh secret and print it once (the old one stops verifying)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source webhook <name> [--rotate] [flags]\n\nPrints the webhook URL, content type and events to paste into the git\nprovider's webhook settings. The secret is only shown with --rotate.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source webhook", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	var (
		gs  gitSourceResource
		err error
	)
	if rotate {
		gs, err = client.RotateGitSourceWebhookSecret(context.Background(), name)
	} else {
		gs, err = client.GetGitSource(context.Background(), name)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get webhook settings for app %q: %w", name, err))
	}
	info := gitSourceWebhookInfo{
		URL:         absoluteWebhookURL(client.BaseURL(), gs.WebhookURL),
		ContentType: "application/json",
		Events:      []string{"push", "pull_request"},
	}
	if rotate {
		info.Secret = gs.WebhookSecret
	}

	if err := renderResult(stdout, of.Format, of.Query, info, func() {
		_, _ = fmt.Fprintf(stdout, "url:           %s\ncontent type:  %s\nevents:        %s\n", info.URL, info.ContentType, strings.Join(info.Events, ", "))
		if info.Secret != "" {
			_, _ = fmt.Fprintf(stdout, "secret:        %s (shown once, will not be shown again)\n", info.Secret)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
