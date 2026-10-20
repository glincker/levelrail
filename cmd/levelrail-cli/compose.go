package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runCompose dispatches "compose <verb>": compose stacks as one unit.
func runCompose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, composeUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, composeUsage(prog))
		return exitOK
	case "deploy":
		return runComposeDeploy(prog, args[1:], stdout, stderr, lookupEnv)
	case "preview":
		return runComposePreview(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runComposeStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "revisions":
		return runComposeRevisions(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runComposeLogs(prog, args[1:], stdout, stderr, lookupEnv)
	case "redeploy":
		return runComposeRedeploy(prog, args[1:], stdout, stderr, lookupEnv)
	case "rollback":
		return runComposeRollback(prog, args[1:], stdout, stderr, lookupEnv)
	case "down":
		return runComposeDown(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown compose command %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, composeUsage(prog))
		return exitUsage
	}
}

func composeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s compose deploy <app> -f compose.yaml [--profile-name P]   deploy or update a stack
  %[1]s compose preview -f compose.yaml                            validate and show what would deploy
  %[1]s compose status <app>                                       per-service status and the stack phase
  %[1]s compose revisions <app>                                    deploy history, newest first
  %[1]s compose logs <app> [--service S] [--since 1h] [-f]         logs for one or every service
  %[1]s compose redeploy <app> [--service S]                       recreate every service, or one
  %[1]s compose rollback <app> [--revision N]                      re-apply an earlier compose file
  %[1]s compose down <app> [--volumes]                             remove the stack (volumes kept by default)

Run "%[1]s compose <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// composeCmd is a parsed compose subcommand: its client, output flags and positional args.
type composeCmd struct {
	client *Client
	of     outputFlags
	json   bool
	args   []string
}

func parseComposeCmd(prog, label, usage string, args []string, want int, stderr io.Writer, lookupEnv func(string) (string, bool), extra func(*flag.FlagSet)) (composeCmd, int, bool) {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	if extra != nil {
		extra(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s\n\nFlags:\n", prog, usage)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return composeCmd{}, exitCode, false
	}
	if len(fs.Args()) != want {
		_, _ = fmt.Fprintf(stderr, "%s: %s takes %d argument(s)\n\n", prog, label, want)
		fs.Usage()
		return composeCmd{}, exitUsage, false
	}
	return composeCmd{client: apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), of: of, json: jsonOut, args: fs.Args()}, 0, true
}

func readComposeFile(file string) ([]byte, error) {
	if file == "" {
		return nil, newValidationError("-f/--file is required (use - for stdin)")
	}
	if file == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(file) //nolint:gosec // operator-supplied CLI flag, same as apps deploy-compose
}

func runComposeDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file string
	var profiles stringList
	cmd, code, ok := parseComposeCmd(prog, "compose deploy", "compose deploy <app> -f compose.yaml [--profile-name P]...", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&file, "file", "", "compose file to deploy (- reads stdin)")
		fs.StringVar(&file, "f", "", "shorthand for --file")
		fs.Var(&profiles, "profile-name", "compose profile to enable (repeatable)")
	})
	if !ok {
		return code
	}
	data, err := readComposeFile(file)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, err)
	}
	res, err := cmd.client.DeployComposeStack(context.Background(), cmd.args[0], data, profiles)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("deploy compose stack %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: revision %d, %d service(s)\n", res.AppID, res.Revision, len(res.Services))
		for _, s := range res.Services {
			_, _ = fmt.Fprintf(stdout, "  %s\t%s\n", s.Name, s.Image)
		}
		if len(res.Skipped) > 0 {
			_, _ = fmt.Fprintf(stdout, "skipped (profile not enabled): %s\n", strings.Join(res.Skipped, ", "))
		}
		for _, n := range res.Notices {
			_, _ = fmt.Fprintf(stdout, "%s: %s\n", n.Level, n.Message)
		}
		_, _ = fmt.Fprintf(stdout, "watch it converge: %s compose status %s\n", prog, res.AppID)
	})
}

func runComposePreview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file string
	var profiles stringList
	cmd, code, ok := parseComposeCmd(prog, "compose preview", "compose preview -f compose.yaml", args, 0, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&file, "file", "", "compose file to check (- reads stdin)")
		fs.StringVar(&file, "f", "", "shorthand for --file")
		fs.Var(&profiles, "profile-name", "compose profile to enable (repeatable)")
	})
	if !ok {
		return code
	}
	data, err := readComposeFile(file)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, err)
	}
	res, err := cmd.client.PreviewCompose(context.Background(), data, profiles)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("preview compose: %w", err))
	}
	code = renderCompose(stdout, stderr, cmd, res, func() { printComposePreview(stdout, res) })
	if code == exitOK && !res.Valid {
		return exitValidation
	}
	return code
}

func printComposePreview(out io.Writer, res apiclient.ComposePreview) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SERVICE\tIMAGE\tPORT\tVOLUMES\tDEPENDS ON\tGENERATED\tREQUIRED")
	for _, s := range res.Services {
		var gen, req []string
		for _, e := range s.Env {
			if e.Generated {
				gen = append(gen, e.Key)
			}
			if e.Required {
				req = append(req, e.Key)
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.Key, s.Image, portOrDash(s.Port), dashJoin(s.Volumes), dashJoin(s.DependsOn), dashJoin(gen), dashJoin(req))
	}
	_ = tw.Flush()
	if len(res.Skipped) > 0 {
		_, _ = fmt.Fprintf(out, "skipped (profile not enabled): %s\n", strings.Join(res.Skipped, ", "))
	}
	if len(res.Profiles) > 0 {
		_, _ = fmt.Fprintf(out, "profiles in this file: %s (enable with --profile-name)\n", strings.Join(res.Profiles, ", "))
	}
	if res.NeedsRoot {
		_, _ = fmt.Fprintln(out, "note: bind-mounts a host directory, which needs the root ability")
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", w)
	}
	for _, e := range res.Errors {
		_, _ = fmt.Fprintf(out, "error: %s\n", e)
	}
	if res.Valid {
		_, _ = fmt.Fprintln(out, "valid")
	}
}

func runComposeStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd, code, ok := parseComposeCmd(prog, "compose status", "compose status <app>", args, 1, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	st, err := cmd.client.GetComposeStack(context.Background(), cmd.args[0])
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose status %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, st, func() {
		_, _ = fmt.Fprintf(stdout, "%s: %s (revision %d)\n", st.AppID, st.Phase, st.Revision)
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SERVICE\tREADY\tREASON\tIMAGE\tDEPENDS ON\tMESSAGE")
		for _, s := range st.Services {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Key, s.Ready, dashIfEmpty(s.Reason), s.Image, dashJoin(s.DependsOn), s.Message)
		}
		_ = tw.Flush()
	})
}

func runComposeRevisions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd, code, ok := parseComposeCmd(prog, "compose revisions", "compose revisions <app>", args, 1, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	revs, err := cmd.client.ListComposeRevisions(context.Background(), cmd.args[0])
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("compose revisions %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, revs, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "REVISION\tSOURCE\tCREATED\tBY\tPROFILES")
		for _, r := range revs {
			_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", r.Revision, r.Source, r.CreatedAt, dashIfEmpty(r.CreatedBy), dashJoin(r.Profiles))
		}
		_ = tw.Flush()
	})
}

func renderCompose(stdout, stderr io.Writer, cmd composeCmd, data any, table func()) int {
	if err := renderResult(stdout, cmd.of.Format, cmd.of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func dashJoin(v []string) string {
	if len(v) == 0 {
		return "-"
	}
	return strings.Join(v, ",")
}

func portOrDash(p int) string {
	if p == 0 {
		return "-"
	}
	return fmt.Sprint(p)
}
