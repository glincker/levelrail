package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// appsListPage is "apps list"'s --max-items/--starting-token output
// shape. Only built when paging is actually requested: the unpaged path
// below keeps printing the bare array ListApps has always returned, so
// a script that doesn't ask for paging sees no change at all.
type appsListPage struct {
	Items      []appResource `json:"items"`
	NextToken  string        `json:"next_token"`
	TotalCount int           `json:"total_count"`
}

// runAppsList implements "apps list": GET /api/v1/apps, printed either
// as a compact table or as JSON. Kept deliberately small (the task this
// command exists for is verifying "apps create" worked, not being a
// full-featured resource browser), per this CLI's own scope: apps
// create is the point, list/get are minimal companions.
//
// --max-items/--starting-token are the one exception: a real fleet can
// run enough apps that fetching all of them every call matters, and
// internal/api's handleListApps already supports limit/offset server
// side (X-Total-Count header and all), so this wires that through
// rather than faking pagination client side.
func runAppsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps list", "print apps as a JSON array to stdout and nothing else", stderr)
	maxItemsFlag := fs.Int("max-items", 0, "cap the number of apps returned by this call (server-side limit, AWS CLI's own --max-items); 0 means every app, same as omitting it")
	environmentFlag := fs.String("environment", "", "only apps in this environment (name or id)")
	startingTokenFlag := fs.String("starting-token", "", "resume from the next_token a previous --max-items call printed, to fetch the next page")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps list [flags]\n\nLists every app the caller's token can read. With --max-items or\n--starting-token, prints {items, next_token, total_count} instead of a\nbare array, so a script can page through a large fleet instead of\nfetching it all in one call.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if *maxItemsFlag == 0 && *startingTokenFlag == "" {
		listApps := client.ListApps
		if *environmentFlag != "" {
			listApps = func(ctx context.Context) ([]appResource, error) {
				return client.ListAppsInEnvironment(ctx, *environmentFlag)
			}
		}
		apps, err := listApps(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
		}
		if err := renderResult(stdout, of.Format, of.Query, apps, func() { printAppsTable(stdout, apps) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}

	offset := 0
	if *startingTokenFlag != "" {
		if *maxItemsFlag == 0 {
			// handleListApps only applies offset when a limit is also
			// set (internal/api/apps_list.go, internal/store/apps_list.go's
			// own `if f.Limit > 0` SQL guard): an offset with no limit
			// would silently be ignored server side and return everything.
			return reportError(stdout, stderr, jsonOut, newValidationError("apps list: --starting-token requires --max-items (the server only pages a result when a limit is set)"))
		}
		n, perr := strconv.Atoi(*startingTokenFlag)
		if perr != nil || n < 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("apps list: --starting-token must be a non-negative integer offset from a previous call's next_token, got %q", *startingTokenFlag))
		}
		offset = n
	}

	page, err := client.ListAppsPage(context.Background(), apiclient.ListAppsOptions{Limit: *maxItemsFlag, Offset: offset, Environment: *environmentFlag})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
	}
	nextToken := ""
	if page.NextOffset > 0 {
		nextToken = strconv.Itoa(page.NextOffset)
	}
	result := appsListPage{Items: page.Items, NextToken: nextToken, TotalCount: page.TotalCount}

	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		printAppsTable(stdout, page.Items)
		if nextToken != "" {
			_, _ = fmt.Fprintf(stdout, "more: rerun with --starting-token %s\n", nextToken)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
