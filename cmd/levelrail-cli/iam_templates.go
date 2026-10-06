package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runIAMTemplates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, iamTemplatesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, iamTemplatesUsage(prog))
		return exitOK
	case "list":
		return runIAMTemplatesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "apply":
		return runIAMTemplatesApply(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown iam templates subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, iamTemplatesUsage(prog))
		return exitUsage
	}
}

func iamTemplatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam templates list [flags]                                                           list ready-made policies
  %[1]s iam templates apply <id> [--param KEY=VALUE] [--name NAME]
                              [--attach-user ID | --attach-token ID] [flags]               create a policy from a template
`, prog)
}

func runIAMTemplatesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "iam templates list", "print templates as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamTemplatesListUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListPolicyTemplates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list policy templates: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() { printPolicyTemplatesTable(stdout, list.Templates) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printPolicyTemplatesTable(out io.Writer, templates []apiclient.PolicyTemplate) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tPARAMS\tDESCRIPTION")
	for _, t := range templates {
		params := make([]string, 0, len(t.Params))
		for _, p := range t.Params {
			params = append(params, p.Name)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.ID, t.Name, strings.Join(params, ","), t.Description)
	}
	_ = tw.Flush()
}

func iamTemplatesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam templates list [flags]

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string     control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string     named credentials profile to read (overrides APP_PROFILE, default "default")
  --json               print templates as JSON to stdout, nothing else
  --output string      output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string       JMESPath expression to filter the result before printing
  -h, --help           show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func parseTemplateParams(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" || v == "" {
			return nil, newValidationError("--param %q must look like KEY=VALUE", p)
		}
		out[k] = v
	}
	return out, nil
}

func runIAMTemplatesApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "iam templates apply", "print the result as JSON to stdout and nothing else", stderr)
	var params stringListFlag
	var name, attachUser, attachToken string
	fs.Var(&params, "param", "template parameter KEY=VALUE (repeatable)")
	fs.StringVar(&name, "name", "", "policy name (default derived from the template)")
	fs.StringVar(&attachUser, "attach-user", "", "attach the new policy to this user ID")
	fs.StringVar(&attachToken, "attach-token", "", "attach the new policy to this token ID")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamTemplatesApplyUsage(prog)) }

	client, id, jsonOut, of, code, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "iam templates apply", "template id"}, lookupEnv)
	if !ok {
		return code
	}
	if attachUser != "" && attachToken != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("use only one of --attach-user and --attach-token"))
	}
	parsed, err := parseTemplateParams(params)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	req := apiclient.ApplyPolicyTemplateRequest{Name: name, Params: parsed}
	switch {
	case attachUser != "":
		req.Attach = &apiclient.ApplyPolicyTemplateAttach{PrincipalType: "user", PrincipalID: attachUser}
	case attachToken != "":
		req.Attach = &apiclient.ApplyPolicyTemplateAttach{PrincipalType: "token", PrincipalID: attachToken}
	}
	res, err := client.ApplyPolicyTemplate(context.Background(), id, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply policy template %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "policy %q (id %s) created from template %s", res.Policy.Name, res.Policy.ID, id)
		if res.Attached {
			_, _ = fmt.Fprint(stdout, " and attached")
		}
		_, _ = fmt.Fprintln(stdout)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func iamTemplatesApplyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam templates apply <id> [flags]

Flags:
  --param string          template parameter KEY=VALUE, repeatable (guest-one-environment needs environment=ID)
  --name string           policy name (default derived from the template)
  --attach-user string    attach the new policy to this user ID
  --attach-token string   attach the new policy to this token ID
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the result as JSON to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
