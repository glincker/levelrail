package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// listCommandParams bundles a read-only, zero-positional-argument
// subcommand's fixed inputs: the "parse the standard api flags, build a
// client, fetch, render" skeleton every such subcommand in this package
// otherwise repeats verbatim (apiFlagSet/parseAPIFlags/apiClientFromFlags/
// reportError/writeScheduledTaskResult, in that order). Shared here
// rather than left inline in each new call site added alongside it
// (settings oauth list, settings email/ingress get, github-app repos,
// gitlab-app projects, bitbucket-app repos, templates list, static-sites
// list, domains certificates): that boilerplate is already duplicated
// dozens of times across this package's older, pre-existing commands,
// but concentrating nine more near-identical copies into one PR is what
// actually trips a duplication check, so it gets factored out here
// instead of repeated a ninth-plus time.
type listCommandParams[T any] struct {
	cmdLabel  string
	jsonUsage string
	usageText string
	fetch     func(*Client, context.Context) (T, error)
	errVerb   string
	print     func(io.Writer, T)
}

// runListCommand runs p's fetch/render skeleton against args.
func runListCommand[T any](prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), p listCommandParams[T]) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, p.cmdLabel, p.jsonUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, p.usageText)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := p.fetch(client, context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", p.errVerb, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { p.print(stdout, result) })
}

// twoArgListParams is listCommandParams' counterpart for a read-only
// subcommand taking exactly two positional arguments (an owner/repo or
// workspace/repo-slug pair): github-app/bitbucket-app's own "branches"
// subcommands share this exact shape.
type twoArgListParams[T any] struct {
	cmdLabel  string
	jsonUsage string
	usageText string
	argsLabel string
	fetch     func(*Client, context.Context, string, string) (T, error)
	// errFmt takes exactly (arg1, arg2, err), matching every call site's
	// own "list branches for %s/%s: %w" shape.
	errFmt string
	print  func(io.Writer, T)
}

func runTwoArgList[T any](prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), p twoArgListParams[T]) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, p.cmdLabel, p.jsonUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, p.usageText)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, p.cmdLabel, p.argsLabel, 2)
	if !ok {
		return exitUsage
	}
	arg1, arg2 := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := p.fetch(client, context.Background(), arg1, arg2)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf(p.errFmt, arg1, arg2, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { p.print(stdout, result) })
}

// twoArgUseAsSourceParams is twoArgListParams' counterpart for the
// "use-as-source" mutation every git provider App integration exposes at
// an owner/repo (or workspace/repo-slug) path: github-app and
// bitbucket-app share this exact shape, differing only in their fetch
// call and response type. gitlab-app takes a single numeric project ID
// instead of two strings, so it stays its own, non-generic
// implementation rather than being forced into this shape.
type twoArgUseAsSourceParams[T any] struct {
	cmdLabel  string
	usageText string
	argsLabel string
	fetch     func(*Client, context.Context, string, string, useRepoAsSourceRequest) (T, error)
	// errFmt takes exactly (arg1, arg2, appName, err).
	errFmt string
	print  func(io.Writer, T)
}

func runTwoArgUseAsSource[T any](prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), p twoArgUseAsSourceParams[T]) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, p.cmdLabel, "print the resulting git source as JSON to stdout and nothing else", stderr)
	req := bindUseAsSourceFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, p.usageText)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, p.cmdLabel, p.argsLabel, 2)
	if !ok {
		return exitUsage
	}
	arg1, arg2 := rest[0], rest[1]
	if req.AppName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires --app-name\n\n", prog, p.cmdLabel)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := p.fetch(client, context.Background(), arg1, arg2, *req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf(p.errFmt, arg1, arg2, req.AppName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { p.print(stdout, result) })
}

// bindUseAsSourceFlags registers the four flags every provider's own
// use-as-source subcommand shares (app-name/branch/build-type/build-path),
// the same request shape internal/api's useRepoAsSourceRequest defines
// for all three providers.
func bindUseAsSourceFlags(fs *flag.FlagSet) *useRepoAsSourceRequest {
	req := &useRepoAsSourceRequest{}
	fs.StringVar(&req.AppName, "app-name", "", "app to connect this repo to (required)")
	fs.StringVar(&req.Branch, "branch", "", "branch to deploy on push (default: the repo's default branch)")
	fs.StringVar(&req.BuildType, "build-type", "", "dockerfile, railpack, or static (default: dockerfile)")
	fs.StringVar(&req.BuildPath, "build-path", "", "path within the repo to build from")
	return req
}
