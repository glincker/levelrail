package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func stackTrustUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s stack trust <command> [flags]

Trusts let a CI workflow swap its OIDC identity for a short-lived token bound to
one stack, so CI needs no stored secret. Root only.

Commands:
  add <name> --repo OWNER/NAME --ref PATTERN [flags]   trust a workflow identity
  list <name>                                          list the trusts of a stack
  remove <name> <id>                                   delete a trust

Run '%[1]s stack trust <command> --help' for the flags of a command.
`, prog)
}

func runStackTrust(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, stackTrustUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "add":
		return stackTrustAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return stackTrustList(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return stackTrustRemove(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s stack trust: unknown command %q\n\n%s", prog, args[0], stackTrustUsage(prog))
	return exitUsage
}

func stackTrustAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var repo, ref, env, subject, verbs *string
	c, code, ok := parseCLICall(prog, "stack trust add", stackCmdUsage(prog, "trust add", "<name> --repo OWNER/NAME --ref PATTERN [--environment E] [--subject PATTERN] [--verbs plan,apply]", `
Flags:
  --repo OWNER/NAME     exact repository (required)
  --ref PATTERN         exact ref or one * per segment, e.g. refs/heads/main or refs/pull/*/merge (required)
  --environment E       exact GitHub environment; required for apply
  --subject PATTERN     optional OIDC subject pattern, must start with repo:OWNER/NAME:
  --verbs LIST          plan, apply or plan,apply (default plan). Apply needs an exact ref and an environment.
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		repo = fs.String("repo", "", "exact repository owner/name")
		ref = fs.String("ref", "", "ref pattern")
		env = fs.String("environment", "", "exact GitHub environment")
		subject = fs.String("subject", "", "OIDC subject pattern")
		verbs = fs.String("verbs", "plan", "plan, apply or plan,apply")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "trust add", "<name> --repo OWNER/NAME --ref PATTERN", 1)
	if !ok {
		return code
	}
	if *repo == "" || *ref == "" {
		return c.invalid("--repo and --ref are required")
	}
	req := apiclient.StackTrustRequest{Repository: *repo, RefPattern: *ref, Environment: *env, SubjectPattern: *subject}
	for _, v := range strings.Split(*verbs, ",") {
		if v = strings.TrimSpace(v); v != "" {
			req.Verbs = append(req.Verbs, v)
		}
	}
	t, raw, err := c.client.AddStackTrust(context.Background(), pos[0], req)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() {
		_, _ = fmt.Fprintf(c.stdout, "trusted %s %s for stack %s with %s (id %s)\n", t.Repository, t.RefPattern, pos[0], strings.Join(t.Verbs, ","), t.ID)
	})
}

func stackTrustList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack trust list", stackCmdUsage(prog, "trust list", "<name>", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "trust list", "<name>", 1)
	if !ok {
		return code
	}
	trusts, raw, err := c.client.ListStackTrusts(context.Background(), pos[0])
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() {
		if len(trusts) == 0 {
			_, _ = fmt.Fprintf(c.stdout, "stack %s has no OIDC trusts\n", pos[0])
			return
		}
		tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tREPOSITORY\tREF\tENVIRONMENT\tVERBS")
		for _, t := range trusts {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Repository, t.RefPattern, t.Environment, strings.Join(t.Verbs, ","))
		}
		_ = tw.Flush()
	})
}

func stackTrustRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack trust remove", stackCmdUsage(prog, "trust remove", "<name> <id>", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "trust remove", "<name> <id>", 2)
	if !ok {
		return code
	}
	if err := c.client.RemoveStackTrust(context.Background(), pos[0], pos[1]); err != nil {
		return iacFail(c, err)
	}
	return c.render(map[string]string{"removed": pos[1]}, func() {
		_, _ = fmt.Fprintf(c.stdout, "removed trust %s from stack %s\n", pos[1], pos[0])
	})
}
