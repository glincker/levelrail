package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func runPagesSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "show" && args[0] != "set") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s pages settings show <name> [flags]\n  %[1]s pages settings set <name> [--spa auto|on|off] [--clean-urls auto|on|off]\n      [--trailing-slash add|remove|auto] [--404 path|auto] [--build-command CMD|auto] [--output-dir DIR|auto]\n      [--password P | --password-stdin | --password-env VAR | --clear-password]\n", prog)
		return exitUsage
	}
	if args[0] == "show" {
		return runPagesSettingsShow(prog, args[1:], stdout, stderr, lookupEnv)
	}
	return runPagesSettingsSet(prog, args[1:], stdout, stderr, lookupEnv)
}

func runPagesSettingsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages settings show", "print the settings as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "pages settings show", "site name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetStaticSiteSettings(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get settings of %q: %w", name, err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { printPagesSettings(stdout, res) })
}

func printPagesSettings(out io.Writer, s apiclient.StaticSiteSettings) {
	tri := func(b *bool) string {
		if b == nil {
			return "auto"
		}
		return strconv.FormatBool(*b)
	}
	or := func(v string) string {
		if v == "" {
			return "auto"
		}
		return v
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SETTING\tOVERRIDE\tSERVING")
	_, _ = fmt.Fprintf(tw, "spa\t%s\t%t\n", tri(s.SPAFallback), s.Effective.SPA)
	_, _ = fmt.Fprintf(tw, "clean-urls\t%s\t%t\n", tri(s.CleanURLs), s.Effective.CleanURLs)
	_, _ = fmt.Fprintf(tw, "trailing-slash\t%s\t%s\n", or(s.TrailingSlash), or(s.Effective.TrailingSlash))
	_, _ = fmt.Fprintf(tw, "404\t%s\t%s\n", or(s.NotFoundPage), or(s.Effective.NotFound))
	_, _ = fmt.Fprintf(tw, "build-command\t%s\t\n", or(s.BuildCommand))
	_, _ = fmt.Fprintf(tw, "output-dir\t%s\t\n", or(s.OutputDir))
	_, _ = fmt.Fprintf(tw, "password\t%s\t\n", map[bool]string{true: "set", false: "none"}[s.Protected])
	_ = tw.Flush()
}

// pagesSettingsPatch maps the flag values given on the command line to a patch;
// "auto" clears an override. given lists the flags the user actually set.
func pagesSettingsPatch(vals map[string]string, given map[string]bool, pwGiven bool) (apiclient.StaticSiteSettingsPatch, error) {
	var p apiclient.StaticSiteSettingsPatch
	str := func(flagName, field string, dst **string) {
		if !given[flagName] {
			return
		}
		if v := vals[flagName]; v == "auto" || v == "" {
			p.Reset = append(p.Reset, field)
		} else {
			*dst = &v
		}
	}
	tri := func(flagName, field string, dst **bool) error {
		if !given[flagName] {
			return nil
		}
		switch vals[flagName] {
		case "auto":
			p.Reset = append(p.Reset, field)
		case "on":
			t := true
			*dst = &t
		case "off":
			f := false
			*dst = &f
		default:
			return newValidationError("--%s must be auto, on or off", flagName)
		}
		return nil
	}
	if err := tri("spa", "spa_fallback", &p.SPAFallback); err != nil {
		return p, err
	}
	if err := tri("clean-urls", "clean_urls", &p.CleanURLs); err != nil {
		return p, err
	}
	str("trailing-slash", "trailing_slash", &p.TrailingSlash)
	str("404", "not_found_page", &p.NotFoundPage)
	str("build-command", "build_command", &p.BuildCommand)
	str("output-dir", "output_dir", &p.OutputDir)
	if len(given) == 0 && !pwGiven {
		return p, newValidationError("pages settings set needs at least one setting flag")
	}
	return p, nil
}

func runPagesSettingsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages settings set", "print the settings as JSON to stdout and nothing else", stderr)
	names := []string{"spa", "clean-urls", "trailing-slash", "404", "build-command", "output-dir"}
	ptrs := map[string]*string{}
	for _, n := range names {
		ptrs[n] = fs.String(n, "", "new value, or auto to clear the override")
	}
	pw := fs.String("password", "", "gate the site with this shared password (stays in shell history, prefer --password-stdin)")
	pwStdin := fs.Bool("password-stdin", false, "read the gate password from the first line of stdin")
	pwEnv := fs.String("password-env", "", "read the gate password from this environment variable")
	clearPW := fs.Bool("clear-password", false, "remove the password gate")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "pages settings set", "site name")
	if !ok {
		return exitUsage
	}
	vals, given := map[string]string{}, map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		if p, ok := ptrs[f.Name]; ok {
			vals[f.Name], given[f.Name] = *p, true
		}
	})
	pwGiven := given["password"] || *pwStdin || *pwEnv != "" || *clearPW
	switch {
	case given["password"]:
		if *pw == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--password is empty, use --clear-password to remove the gate"))
		}
		vals["password"] = *pw
	case *pwStdin:
		line, rerr := bufio.NewReader(os.Stdin).ReadString('\n')
		if rerr != nil && line == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--password-stdin found no password on stdin"))
		}
		vals["password"] = strings.TrimRight(line, "\r\n")
	case *pwEnv != "":
		v, found := lookupEnv(*pwEnv)
		if !found || v == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--password-env: %s is not set", *pwEnv))
		}
		vals["password"] = v
	}
	if pwGiven && *clearPW && len(vals["password"]) > 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--clear-password cannot be combined with a password"))
	}
	patch, err := pagesSettingsPatch(vals, given, pwGiven)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if pwGiven {
		patch.Password = ptrString(vals["password"])
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.PatchStaticSiteSettings(context.Background(), name, patch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update settings of %q: %w", name, err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { printPagesSettings(stdout, res) })
}

func runStaticRoutesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages routes list", "print the rules as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "pages routes list", "site name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetStaticSiteRules(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list redirects of %q: %w", name, err))
	}
	return renderOrFail(stdout, stderr, of, res.UI.Redirects, func() { printUIRedirects(stdout, res.UI.Redirects) })
}

func printUIRedirects(out io.Writer, rules []staticroutes.UIRedirect) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "INDEX\tFROM\tTO\tSTATUS\tFORCE")
	for i, r := range rules {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%t\n", i, r.From, r.To, r.Status, r.Force)
	}
	_ = tw.Flush()
}

func runStaticRoutesAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages routes add", "print the rules as JSON to stdout and nothing else", stderr)
	status := fs.Int("status", 0, "HTTP status, 301 when omitted")
	force := fs.Bool("force", false, "apply the rule even when a file exists at the path")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "pages routes add", "a site name, a from path and a to target", 3)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := client.GetStaticSiteRules(ctx, rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read redirects of %q: %w", rest[0], err))
	}
	body := apiclient.StaticSiteRulesBody{Redirects: append(cur.UI.Redirects, staticroutes.UIRedirect{From: rest[1], To: rest[2], Status: *status, Force: *force}), Headers: cur.UI.Headers}
	return putPagesRules(ctx, client, rest[0], body, stdout, stderr, of, jsonOut)
}

func runStaticRoutesRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages routes rm", "print the rules as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "pages routes rm", "a site name and an index or from path", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := client.GetStaticSiteRules(ctx, rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read redirects of %q: %w", rest[0], err))
	}
	idx := -1
	if n, convErr := strconv.Atoi(rest[1]); convErr == nil && n >= 0 && n < len(cur.UI.Redirects) {
		idx = n
	} else {
		for i, r := range cur.UI.Redirects {
			if r.From == rest[1] {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("no dashboard redirect %q on site %q (see routes list)", rest[1], rest[0]))
	}
	kept := append(append([]staticroutes.UIRedirect{}, cur.UI.Redirects[:idx]...), cur.UI.Redirects[idx+1:]...)
	return putPagesRules(ctx, client, rest[0], apiclient.StaticSiteRulesBody{Redirects: kept, Headers: cur.UI.Headers}, stdout, stderr, of, jsonOut)
}

func putPagesRules(ctx context.Context, client *Client, name string, body apiclient.StaticSiteRulesBody, stdout, stderr io.Writer, of outputFlags, jsonOut bool) int {
	res, err := client.PutStaticSiteRules(ctx, name, body)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save redirects of %q: %w", name, err))
	}
	return renderOrFail(stdout, stderr, of, res.UI.Redirects, func() { printUIRedirects(stdout, res.UI.Redirects) })
}

func ptrString(v string) *string { return &v }
