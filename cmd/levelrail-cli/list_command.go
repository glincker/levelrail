package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// listCommandParams holds the fixed inputs of a read-only, zero-argument list subcommand.
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

// gitlab-app takes a numeric project ID, so it does not share this shape.
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

func bindUseAsSourceFlags(fs *flag.FlagSet) *useRepoAsSourceRequest {
	req := &useRepoAsSourceRequest{}
	fs.StringVar(&req.AppName, "app-name", "", "app to connect this repo to (required)")
	fs.StringVar(&req.Branch, "branch", "", "branch to deploy on push (default: the repo's default branch)")
	fs.StringVar(&req.BuildType, "build-type", "", "dockerfile, railpack, or static (default: dockerfile)")
	fs.StringVar(&req.BuildPath, "build-path", "", "path within the repo to build from")
	fs.StringVar(&req.TriggerMode, "trigger-mode", "", "push or release (default: push); release deploys only on a tag push or a published github release")
	return req
}
