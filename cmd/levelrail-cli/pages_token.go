package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ttlToDays turns "30d" or a Go duration such as "720h" into whole days, rounding up.
func ttlToDays(v string) (int, error) {
	if n, ok := strings.CutSuffix(v, "d"); ok {
		d, err := strconv.Atoi(n)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("invalid --ttl %q", v)
		}
		return d, nil
	}
	dur, err := time.ParseDuration(v)
	if err != nil || dur <= 0 {
		return 0, fmt.Errorf("invalid --ttl %q, use for example 30d or 720h", v)
	}
	return int((dur + 24*time.Hour - 1) / (24 * time.Hour)), nil
}

const pagesCiUsage = "Usage:\n  %[1]s pages token create [--label L] [--ttl 30d] <site | --site S> [flags]\n  %[1]s pages token list <site> [flags]\n  %[1]s pages token revoke <site> <token-id> [flags]\n"

// runPagesToken manages deploy tokens that can only deploy one Pages site, for CI pipelines.
func runPagesToken(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "create" && args[0] != "list" && args[0] != "revoke") {
		_, _ = fmt.Fprintf(stderr, pagesCiUsage, prog)
		return exitUsage
	}
	verb := args[0]
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages token "+verb, "print the result as JSON to stdout and nothing else", stderr)
	label := fs.String("label", "", "name of the token, for example the pipeline that uses it")
	days := fs.Int("expires-days", 0, "days until the token expires (default: the server setting)")
	siteFlag := fs.String("site", "", "site name, instead of the positional argument")
	ttl := fs.String("ttl", "", "lifetime such as 30d or 720h, capped by the server setting")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	if *siteFlag != "" {
		pos = append([]string{*siteFlag}, pos...)
	}
	want := 1
	if verb == "revoke" {
		want = 2
	}
	if *ttl != "" {
		d, err := ttlToDays(*ttl)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		*days = d
	}
	if len(pos) != want {
		_, _ = fmt.Fprintf(stderr, pagesCiUsage, prog)
		return exitUsage
	}
	site := pos[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch verb {
	case "revoke":
		if err := client.RevokeSiteDeployToken(ctx, site, pos[1]); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke token of %q: %w", site, err))
		}
		_, _ = fmt.Fprintf(stdout, "token %s of %s revoked\n", pos[1], site)
		return exitOK
	case "list":
		list, err := client.ListSiteDeployTokens(ctx, site)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tokens of %q: %w", site, err))
		}
		return renderOrFail(stdout, stderr, of, list, func() {
			for _, t := range list {
				state := "active"
				if t.RevokedAt != nil {
					state = "revoked"
				}
				_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", t.ID, t.Name, state)
			}
		})
	}
	t, err := client.CreateSiteDeployToken(ctx, site, *label, *days)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create token for %q: %w", site, err))
	}
	return renderOrFail(stdout, stderr, of, t, func() {
		_, _ = fmt.Fprintf(stdout, "%s\n", t.Token)
		_, _ = fmt.Fprintln(stderr, "This token is shown once. It can only deploy this site. Set it as APP_API_TOKEN in your pipeline.")
	})
}
