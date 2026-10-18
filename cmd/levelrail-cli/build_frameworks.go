package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func buildFrameworksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s build frameworks [flags]                list the framework presets (port, health path, build mode)
  %[1]s build frameworks ID [flags]             show one preset with its env hints and volumes
  %[1]s build frameworks ID --dockerfile        print the tuned Dockerfile template for a preset

Use a preset when creating an app: %[1]s apps create --framework ID ...

Flags:
  --dockerfile         print the Dockerfile template (needs ID)
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string     control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string     named credentials profile to read
  --json               print the result as JSON to stdout, nothing else
  --output string      output format: json, table, or text
  --query string       JMESPath expression to filter the result before printing
  -h, --help           show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runBuildFrameworks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "build frameworks", "print the result as JSON to stdout and nothing else", stderr)
	var dockerfile bool
	fs.BoolVar(&dockerfile, "dockerfile", false, "print the Dockerfile template for the preset")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, buildFrameworksUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprintf(stderr, "%s: build frameworks takes at most one preset id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	id := fs.Arg(0)
	if dockerfile && id == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--dockerfile needs a preset id"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	switch {
	case dockerfile:
		text, err := client.GetFrameworkDockerfile(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get dockerfile for %q: %w", id, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": id, "dockerfile": text}, func() {
			_, _ = fmt.Fprint(stdout, text)
		})
	case id != "":
		p, err := client.GetFramework(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get framework %q: %w", id, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, p, func() {
			_, _ = fmt.Fprintf(stdout, "%s (%s)\nbuild:    %s, output %s\nport:     %d\nhealth:   %s\n", p.Name, p.ID, p.BuildType, p.OutputMode, p.Port, p.HealthPath)
			for _, e := range p.EnvHints {
				req := ""
				if e.Required {
					req = " (required)"
				}
				_, _ = fmt.Fprintf(stdout, "env:      %s%s  %s\n", e.Key, req, e.Description)
			}
			for _, v := range p.Volumes {
				_, _ = fmt.Fprintf(stdout, "volume:   %s  %s\n", v.Path, v.Purpose)
			}
		})
	}

	list, err := client.ListFrameworks(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list frameworks: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() {
		for _, p := range list {
			port := "-"
			if p.Port > 0 {
				port = fmt.Sprint(p.Port)
			}
			_, _ = fmt.Fprintf(stdout, "%-15s %-24s %-9s port %-5s health %s\n", p.ID, p.Name, p.BuildType, port, strings.TrimSpace(p.HealthPath))
		}
	})
}
