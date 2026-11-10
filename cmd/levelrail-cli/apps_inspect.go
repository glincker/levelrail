package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var appsInspectViews = []string{"effective-config", "pending-changes", "failure-context", "rollback-targets", "image-status"}

func isAppsInspectView(s string) bool {
	for _, v := range appsInspectViews {
		if v == s {
			return true
		}
	}
	return false
}

// runAppsInspect implements "apps inspect <app> <view>", read-only views of
// what the dashboard cards show. --json prints the API body unchanged.
func runAppsInspect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps inspect", "print the API response body to stdout unchanged and nothing else", stderr)
	var keys []string
	var contextName, branch string
	fs.Func("key", "effective-config only: show just this env key (repeatable or comma separated)", func(v string) error {
		for _, k := range strings.Split(v, ",") {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
		return nil
	})
	fs.StringVar(&contextName, "context", "", "effective-config only: deploy context to resolve (default production)")
	fs.StringVar(&branch, "branch", "", "effective-config only: branch used to match branch contexts")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsInspectUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps inspect requires an app name and a view\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, sub := rest[0], rest[1]
	// Completion offers the view right after "inspect", so accept that order too.
	if !isAppsInspectView(sub) && isAppsInspectView(name) {
		name, sub = sub, name
	}
	q := url.Values{}
	switch sub {
	case "effective-config":
		if contextName != "" {
			q.Set("context", contextName)
		}
		if branch != "" {
			q.Set("branch", branch)
		}
	case "pending-changes", "failure-context", "rollback-targets", "image-status":
		if len(keys) > 0 || contextName != "" || branch != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--key, --context and --branch apply only to effective-config"))
		}
	default:
		return reportError(stdout, stderr, jsonOut, newValidationError("unknown view %q, want one of %s", sub, strings.Join(appsInspectViews, ", ")))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	body, err := client.GetAppDiagnostic(context.Background(), name, sub, q)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get %s for app %q: %w", sub, name, err))
	}
	if len(keys) > 0 {
		if body, err = filterEffectiveConfigKeys(body, keys); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	return writeAppsInspectResult(stdout, stderr, of, sub, body)
}

func writeAppsInspectResult(stdout, stderr io.Writer, of outputFlags, view string, body json.RawMessage) int {
	if of.Format == outputJSON && of.Query == "" {
		if _, err := fmt.Fprintln(stdout, string(bytes.TrimSpace(body))); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	var generic any
	if err := json.Unmarshal(body, &generic); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("decode %s response: %w", view, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, generic, func() {
		if view == "effective-config" {
			var eff apiclient.EffectiveConfig
			if err := json.Unmarshal(body, &eff); err == nil {
				printEffectiveConfigTable(stdout, eff)
				return
			}
		}
		printInspectFields(stdout, view, generic)
	})
}

// filterEffectiveConfigKeys keeps only the named env entries and passes every
// other field through as raw JSON, so masked values stay exactly as sent.
func filterEffectiveConfigKeys(body json.RawMessage, keys []string) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode effective-config response: %w", err)
	}
	var env []json.RawMessage
	if raw, ok := doc["env"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode effective-config env: %w", err)
		}
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	found := map[string]bool{}
	kept := make([]json.RawMessage, 0, len(keys))
	for _, e := range env {
		var head struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(e, &head); err == nil && want[head.Key] {
			kept = append(kept, e)
			found[head.Key] = true
		}
	}
	var missing []string
	for _, k := range keys {
		if !found[k] {
			missing = append(missing, strconv.Quote(safeTerminalText(k)))
		}
	}
	if len(missing) > 0 {
		return nil, newValidationError("env key %s is not in the effective config", strings.Join(missing, ", "))
	}
	envJSON, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("encode filtered env: %w", err)
	}
	doc["env"] = envJSON
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode filtered effective config: %w", err)
	}
	return out, nil
}

func printEffectiveConfigTable(out io.Writer, eff apiclient.EffectiveConfig) {
	_, _ = fmt.Fprintf(out, "context: %s\n", safeTerminalText(eff.Context))
	if len(eff.Env) == 0 {
		_, _ = fmt.Fprintln(out, "no env vars")
	} else {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSECRET\tSOURCE")
		for _, e := range eff.Env {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%s\n", oneLine(e.Key, 80), dashIfEmpty(oneLine(e.Value, 80)), e.Secret, dashIfEmpty(oneLine(e.Source, 40)))
		}
		_ = tw.Flush()
	}
	printEffectiveSetting(out, "build_command", eff.BuildCommand)
	printEffectiveSetting(out, "publish_dir", eff.PublishDir)
	names := make([]string, 0, len(eff.BuildArgs))
	for n := range eff.BuildArgs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		printEffectiveSetting(out, "build_arg "+n, eff.BuildArgs[n])
	}
}

func printEffectiveSetting(out io.Writer, label string, s apiclient.EffectiveSetting) {
	if s.Value == "" && s.Source == "" {
		return
	}
	_, _ = fmt.Fprintf(out, "%s: %s (%s)\n", safeTerminalText(label), dashIfEmpty(oneLine(s.Value, 120)), dashIfEmpty(safeTerminalText(s.Source)))
}

// printInspectFields prints one "path: value" line per leaf; every byte from
// the server goes through safeTerminalText so it cannot steer the terminal.
func printInspectFields(out io.Writer, view string, v any) {
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			_, _ = fmt.Fprintf(out, "no %s\n", strings.ReplaceAll(view, "-", " "))
			return
		}
	case map[string]any:
		if len(t) == 0 {
			_, _ = fmt.Fprintf(out, "no %s\n", strings.ReplaceAll(view, "-", " "))
			return
		}
	}
	walkInspectFields(out, "", v)
}

func walkInspectFields(out io.Writer, prefix string, v any) {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			writeInspectLine(out, prefix, "{}")
			return
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			walkInspectFields(out, p, t[k])
		}
	case []any:
		if len(t) == 0 {
			writeInspectLine(out, prefix, "[]")
			return
		}
		for i, e := range t {
			walkInspectFields(out, prefix+"["+strconv.Itoa(i)+"]", e)
		}
	case nil:
		writeInspectLine(out, prefix, "-")
	default:
		writeInspectLine(out, prefix, scalarText(t))
	}
}

func writeInspectLine(out io.Writer, path, value string) {
	value = safeTerminalText(value)
	if path == "" {
		_, _ = fmt.Fprintln(out, value)
		return
	}
	_, _ = fmt.Fprintf(out, "%s: %s\n", safeTerminalText(path), value)
}

func appsInspectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps inspect <app> effective-config [--key K] [--context C] [--branch B] [flags]
  %[1]s apps inspect <app> pending-changes [flags]
  %[1]s apps inspect <app> failure-context [flags]
  %[1]s apps inspect <app> rollback-targets [flags]
  %[1]s apps inspect <app> image-status [flags]

Read-only diagnostics, the same data the dashboard cards show. Nothing is
changed. Secret values stay masked exactly as the API returns them.

Views:
  effective-config   resolved env and build settings for a deploy, with the source of each
  pending-changes    saved settings not yet applied by a deploy
  failure-context    last lines of the failing container, or the failed deploy's build log
  rollback-targets   pinned releases image cleanup never removes, newest first
  image-status       whether the image tag moved and its architecture matches the node

Flags:
  --key string             effective-config only: show just this env key (repeatable or comma separated)
  --context string         effective-config only: deploy context to resolve (default production)
  --branch string          effective-config only: branch used to match branch contexts
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   print the API response body unchanged to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
