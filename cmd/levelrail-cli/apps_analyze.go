package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsAnalyze prints what the platform detects in a repository and why,
// without creating anything.
func runAppsAnalyze(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps analyze", "print the detection result as JSON to stdout and nothing else", stderr)
	var branch, dir string
	fs.StringVar(&branch, "branch", "", "branch to inspect (default: main, then master)")
	fs.StringVar(&dir, "dir", "", "app directory inside the repository, for a monorepo")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps analyze <repo-url> [flags]\n\nExplains what would be detected for a repository (framework, build type,\nport, runtime version, env variables) and the signals behind each choice.\nNothing is created.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, repo, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps analyze", "repository URL"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.AnalyzeGitSource(context.Background(), apiclient.ImportAnalyzeRequest{RepoURL: repo, Ref: branch, Directory: dir})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("analyze repository: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printDetection(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDetection(out io.Writer, a apiclient.ImportAnalysis) {
	d := a.Detection
	line := func(format string, v ...any) { _, _ = fmt.Fprintf(out, format+"\n", v...) }
	line("Framework:  %s (%s)", dash(d.Framework), dash(d.Language))
	line("Build type: %s", d.BuildType)
	line("Confidence: %s", d.Confidence)
	if d.Port.Value > 0 {
		line("Port:       %d (%s)", d.Port.Value, d.Port.Source)
	} else {
		line("Port:       -")
	}
	line("Health:     %s", d.HealthPath)
	if d.StartCommand != "" {
		line("Start:      %s", d.StartCommand)
	}
	if d.BuildCommand != "" {
		line("Build:      %s", d.BuildCommand)
	}
	if d.OutputDir != "" {
		line("Output dir: %s", d.OutputDir)
	}
	if d.SPA {
		line("SPA mode:   on (unknown paths serve index.html)")
	}
	if d.RuntimeVersion.Value != "" {
		line("Runtime:    %s (%s)", d.RuntimeVersion.Value, d.RuntimeVersion.Source)
	}
	if len(d.Reasons) > 0 {
		line("\nWhy:")
		for _, r := range d.Reasons {
			line("  - [%s] %s", r.Signal, r.Detail)
		}
	}
	if len(d.EnvVars) > 0 {
		line("\nEnvironment:")
		for _, e := range d.EnvVars {
			tags := []string{}
			if e.Required {
				tags = append(tags, "required")
			}
			if e.SecretGuess {
				tags = append(tags, "secret")
			}
			line("  - %s %s(%s)", e.Name, tagList(tags), e.Source)
		}
	}
	if len(a.Packages) > 0 {
		line("\nWorkspace packages:")
		for _, p := range a.Packages {
			line("  - %s (%s)", p.Name, p.Dir)
		}
	}
	for _, w := range d.Warnings {
		line("warning: %s", w.Message)
	}
}

func tagList(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return strings.Join(tags, ", ") + " "
}
