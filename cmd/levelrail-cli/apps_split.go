package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type repeatedFlag []string

func (r *repeatedFlag) String() string     { return strings.Join(*r, ",") }
func (r *repeatedFlag) Set(v string) error { *r = append(*r, v); return nil }

func runAppsSplit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsSplitUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsSplitUsage(prog))
		return exitOK
	case "start", "status", "weights", "stop", "promote":
		return runAppsSplitVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps split subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsSplitUsage(prog))
		return exitUsage
	}
}

func appsSplitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps split start <app-name> --variant branch:<branch>=<percent> [--variant release:<deploy>=<percent>] [--cookie-name n] [--cookie-ttl 720h]
                                                      send a share of visitors to a branch deploy or a retained release
  %[1]s apps split status <app-name>                  show variants, weights, request counts and QA links
  %[1]s apps split weights <app-name> --set <variant>=<percent> ...   change weights, all must add up to 100
  %[1]s apps split promote <app-name> --variant <id-or-label>   make a variant the production deploy, then end the test
  %[1]s apps split stop <app-name>                    end the test, every visitor gets production

Production gets the leftover percent when it is not listed. Visitors keep
their variant through a cookie (HttpOnly, set only while a test runs). Open a
variant's QA link to force it for yourself. Needs a container app with a
domain; static sites are not supported yet. Promote is refused while
publishing is locked.
`, prog)
}

func runAppsSplitVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps split "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var variants, sets repeatedFlag
	fs.Var(&variants, "variant", "start: kind:ref=percent (repeatable); promote: the variant id or label")
	fs.Var(&sets, "set", "weights: variant=percent (repeatable)")
	cookieName := fs.String("cookie-name", "", "start: name of the sticky cookie (default from APP_SPLIT_COOKIE_NAME)")
	cookieTTL := fs.String("cookie-ttl", "", "start: lifetime of the sticky cookie, such as 720h")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, appsSplitUsage(prog))
		_, _ = fmt.Fprintf(stderr, "\nFlags for apps split %s:\n", verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps split "+verb, "app name")
	if !ok {
		return exitUsage
	}
	req := apiclient.SplitStartRequest{CookieName: *cookieName, CookieTTL: *cookieTTL}
	var weights map[string]int
	var err error
	switch verb {
	case "start":
		if req.Variants, err = parseSplitVariants(variants); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: apps split start: %v\n", prog, err)
			return exitUsage
		}
	case "weights":
		if weights, err = parseSplitWeights(sets); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: apps split weights: %v\n", prog, err)
			return exitUsage
		}
	case "promote":
		if len(variants) != 1 {
			_, _ = fmt.Fprintf(stderr, "%s: apps split promote requires one --variant\n", prog)
			return exitUsage
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var result apiclient.SplitTest
	switch verb {
	case "start":
		result, err = client.StartSplit(ctx, appName, req)
	case "weights":
		result, err = client.SetSplitWeights(ctx, appName, weights)
	case "promote":
		result, err = client.PromoteSplit(ctx, appName, variants[0])
	case "stop":
		result, err = client.StopSplit(ctx, appName)
	default:
		result, err = client.GetSplit(ctx, appName)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("split %s for app %q: %w", verb, appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printSplit(stdout, result) })
}

func parseSplitVariants(in []string) ([]apiclient.SplitVariantInput, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("at least one --variant kind:ref=percent is required")
	}
	out := make([]apiclient.SplitVariantInput, 0, len(in))
	for _, v := range in {
		spec, pct, ok := strings.Cut(v, "=")
		kind, ref, ok2 := strings.Cut(spec, ":")
		w, err := strconv.Atoi(strings.TrimSpace(pct))
		if !ok || !ok2 || err != nil || kind == "" || ref == "" {
			return nil, fmt.Errorf("--variant %q must look like branch:redesign=30", v)
		}
		out = append(out, apiclient.SplitVariantInput{Kind: kind, Ref: ref, Weight: w})
	}
	return out, nil
}

func parseSplitWeights(in []string) (map[string]int, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("at least one --set variant=percent is required")
	}
	out := make(map[string]int, len(in))
	for _, v := range in {
		name, pct, ok := strings.Cut(v, "=")
		w, err := strconv.Atoi(strings.TrimSpace(pct))
		if !ok || err != nil || name == "" {
			return nil, fmt.Errorf("--set %q must look like production=70", v)
		}
		out[name] = w
	}
	return out, nil
}

func printSplit(out io.Writer, r apiclient.SplitTest) {
	if !r.Supported {
		_, _ = fmt.Fprintln(out, r.Unsupported)
		return
	}
	if r.TestID == "" {
		_, _ = fmt.Fprintln(out, "no split test has run for this app")
		return
	}
	state := r.Status
	if !r.Active && r.EndReason != "" {
		state += " (" + r.EndReason + ")"
	}
	_, _ = fmt.Fprintf(out, "split test: %s\ncookie:     %s\n", state, r.CookieName)
	if r.CreatedAt != nil {
		_, _ = fmt.Fprintf(out, "started:    %s\n", r.CreatedAt.Format(time.RFC3339))
	}
	for _, v := range r.Variants {
		_, _ = fmt.Fprintf(out, "  %-14s %3d%%  %-11s %7d requests (%.0f%%)  id %s\n", v.Label, v.Weight, v.Status, v.Requests, v.Share*100, v.ID)
		if v.QALink != "" {
			_, _ = fmt.Fprintf(out, "    QA link: %s\n", v.QALink)
		}
	}
}
