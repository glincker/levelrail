package main

import (
	"context"
	"fmt"
	"io"
)

const pagesHookUsage = "Usage:\n  %[1]s pages hook create|rotate|rm|show <site> [flags]\n"

// runPagesHook manages the secret deploy hook URL of a git-connected Pages site.
func runPagesHook(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, pagesHookUsage, prog)
		return exitUsage
	}
	verb, args := args[0], args[1:]
	if verb != "create" && verb != "rotate" && verb != "rm" && verb != "show" {
		_, _ = fmt.Fprintf(stderr, pagesHookUsage, prog)
		return exitUsage
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages hook "+verb, "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	if len(pos) != 1 {
		_, _ = fmt.Fprintf(stderr, pagesHookUsage, prog)
		return exitUsage
	}
	site := pos[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch verb {
	case "rm":
		if err := client.DeleteStaticSiteHook(ctx, site); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke hook of %q: %w", site, err))
		}
		_, _ = fmt.Fprintf(stdout, "deploy hook of %s revoked\n", site)
		return exitOK
	case "show":
		h, err := client.GetStaticSiteHook(ctx, site)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read hook of %q: %w", site, err))
		}
		return renderOrFail(stdout, stderr, of, h, func() {
			if !h.Enabled {
				_, _ = fmt.Fprintf(stdout, "no deploy hook on %s\n", site)
				return
			}
			_, _ = fmt.Fprintf(stdout, "deploy hook enabled, created %s\n", h.CreatedAt.Format("2006-01-02 15:04"))
		})
	}
	if verb == "create" {
		cur, err := client.GetStaticSiteHook(ctx, site)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read hook of %q: %w", site, err))
		}
		if cur.Enabled {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("%q already has a deploy hook, use pages hook rotate to replace it", site))
		}
	}
	h, err := client.PutStaticSiteHook(ctx, site)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s hook of %q: %w", verb, site, err))
	}
	return renderOrFail(stdout, stderr, of, h, func() {
		_, _ = fmt.Fprintf(stdout, "%s\n", h.URL)
		_, _ = fmt.Fprintln(stderr, "This URL is shown once. POST to it to redeploy the site's branch.")
	})
}
