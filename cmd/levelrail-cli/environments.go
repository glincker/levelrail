package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runEnvironments dispatches "environments list|create|update|delete", the
// instance-wide typed environments (dev, test, uat, production, custom).
func runEnvironments(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, environmentsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, environmentsUsage(prog))
		return exitOK
	case "list":
		return runEnvironmentsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runEnvironmentsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runEnvironmentsUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runEnvironmentsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown environments subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, environmentsUsage(prog))
		return exitUsage
	}
}

func environmentsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s environments list [flags]                       list every environment with its kind and app counts
  %[1]s environments create --name NAME [--kind KIND] [--protected] [--sort-order N] [flags]
  %[1]s environments update <id> [--name NAME] [--kind KIND] [--sort-order N] [--protected=true|false] [flags]
  %[1]s environments delete <id> [--move-to ID] [flags]

KIND is one of dev, test, uat, production, custom. The built-in
environments (env_dev, env_test, env_uat, env_production) cannot be
deleted. Deleting an environment that still holds apps or databases needs
--move-to ID; a protected environment cannot take part in that bulk move.
Move one app with "%[1]s apps move-env" and one database with
"%[1]s databases move-env".
`, prog)
}

func runEnvironmentsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "environments list", "print environments as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s environments list [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	envs, err := client.ListAllEnvironments(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list environments: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, envs, func() { printGlobalEnvironmentsTable(stdout, envs) })
}

func printGlobalEnvironmentsTable(out io.Writer, envs []apiclient.GlobalEnvironmentResource) {
	if len(envs) == 0 {
		_, _ = fmt.Fprintln(out, "no environments")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tSCOPE\tPROTECTED\tAPPS\tDATABASES")
	for _, e := range envs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%d\t%d\n", e.ID, e.Name, e.Kind, e.Scope, e.Protected, e.AppCount, e.DatabaseCount)
	}
	_ = tw.Flush()
}

func runEnvironmentsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "environments create", "print the created environment as JSON to stdout and nothing else", stderr)
	var req apiclient.CreateGlobalEnvironmentRequest
	fs.StringVar(&req.Name, "name", "", "environment name (required)")
	fs.StringVar(&req.Kind, "kind", "custom", "dev, test, uat, production or custom")
	fs.BoolVar(&req.Protected, "protected", false, "require approval for deploys and moves that touch this environment")
	fs.IntVar(&req.SortOrder, "sort-order", 0, "position in lists, lower first")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s environments create --name NAME [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if req.Name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateGlobalEnvironment(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create environment %q: %w", req.Name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q created (id %s, kind %s)\n", created.Name, created.ID, created.Kind)
	})
}

func runEnvironmentsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "environments update", "print the updated environment as JSON to stdout and nothing else", stderr)
	name := fs.String("name", "", "new name")
	kind := fs.String("kind", "", "dev, test, uat, production or custom")
	sortOrder := fs.Int("sort-order", 0, "position in lists, lower first")
	protected := fs.Bool("protected", false, "protect or unprotect (pass --protected=false to clear)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s environments update <id> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	id, tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseEnvironmentIDCommand(fs, args, stderr, prog, "environments update", apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP})
	if !ok {
		return exitCode
	}
	var patch apiclient.PatchEnvironmentRequest
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "name":
			patch.Name = name
		case "kind":
			patch.Kind = kind
		case "sort-order":
			patch.SortOrder = sortOrder
		case "protected":
			patch.Protected = protected
		}
	})
	if patch == (apiclient.PatchEnvironmentRequest{}) {
		return reportError(stdout, stderr, jsonOut, newValidationError("environments update needs at least one of --name, --kind, --sort-order, --protected"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.PatchEnvironment(context.Background(), id, patch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update environment %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q updated (kind %s, protected %t)\n", updated.Name, updated.Kind, updated.Protected)
	})
}

func runEnvironmentsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "environments delete", "print the result as JSON to stdout and nothing else", stderr)
	moveTo := fs.String("move-to", "", "retag every app and database to this environment id before deleting")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s environments delete <id> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	id, tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseEnvironmentIDCommand(fs, args, stderr, prog, "environments delete", apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP})
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteEnvironmentMoveTo(context.Background(), id, *moveTo); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete environment %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"deleted": id}, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q deleted\n", id)
	})
}
