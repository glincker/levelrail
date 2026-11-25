package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// topWhyTemplates bounds the text output; --json carries every cluster.
const topWhyTemplates = 5

func runDeploymentsWhy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "deploys why", "print the verdict as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, "Usage:\n  %s deploys why <deploy-id> [flags]\n", prog) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "deploys why", "a deploy id", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	why, err := client.DeployWhy(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy why: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, why, func() { printDeployWhy(stdout, why) })
}

func printDeployWhy(out io.Writer, w apiclient.DeployWhy) {
	_, _ = fmt.Fprintf(out, "deploy %s: %s\n", w.DeployID, w.Verdict)
	if w.ExitCode != nil {
		_, _ = fmt.Fprintf(out, "  exit code: %d\n", *w.ExitCode)
	}
	if w.Health != nil {
		_, _ = fmt.Fprintf(out, "  last health: %s ok=%t status=%d %s\n", cleanWhyText(w.Health.Target), w.Health.Success, w.Health.StatusCode, cleanWhyText(w.Health.Error))
	}
	_, _ = fmt.Fprintf(out, "  new log patterns vs %s: %d\n", w.BaselineID, w.NewTemplateCount)
	for i, t := range w.NewTemplates {
		if i == topWhyTemplates {
			break
		}
		_, _ = fmt.Fprintf(out, "    %dx %s\n", t.Count, cleanWhyText(t.Sample))
	}
}

var ansiEscRe = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)?|[@-Z\\-_])`)

// cleanWhyText drops terminal escapes and control characters from log text so
// a stored sample cannot repaint or hijack the operator's terminal.
func cleanWhyText(s string) string {
	s = ansiEscRe.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
