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

// runRoles dispatches "roles <verb>": list, create, update, delete (experimental access-roles).
func runRoles(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, rolesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, rolesUsage(prog))
		return exitOK
	case "list":
		return runRolesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runRolesCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runRolesUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runRolesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown roles subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, rolesUsage(prog))
		return exitUsage
	}
}

func rolesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s roles list [flags]                                             list stored roles with their user counts
  %[1]s roles create <name> --abilities LIST [--visibility all|granted] [--description TEXT] [flags]   create a custom role
  %[1]s roles update <role> [--name N] [--abilities LIST] [--visibility V] [--description T] [flags]   edit a custom role
  %[1]s roles delete <role> [flags]                                    delete an unused custom role

<role> is a role id or name. Run "%[1]s roles <subcommand> -h" for its flags.
`, prog)
}

func printStoredRolesTable(out io.Writer, roles []roleResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tVISIBILITY\tBUILTIN\tUSERS\tABILITIES")
	for _, r := range roles {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%d\t%s\n", r.ID, r.Name, r.Visibility, r.Builtin, r.UserCount, strings.Join(r.Abilities, ","))
	}
	_ = tw.Flush()
}

// findRole resolves a role reference, an id or a name, against the stored roles.
func findRole(ctx context.Context, client *apiclient.Client, ref string) (roleResource, error) {
	roles, err := client.ListRoles(ctx)
	if err != nil {
		return roleResource{}, fmt.Errorf("list roles: %w", err)
	}
	for _, r := range roles {
		if r.ID == ref || r.Name == ref {
			return r, nil
		}
	}
	return roleResource{}, newValidationError("no role with id or name %q (see \"roles list\")", ref)
}

const rolesFlagsHelp = `
Flags:
  --token string        API token (default: %[2]s env var, then the credentials file)
  --api-url string      control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string      named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                print the result as JSON to stdout, nothing else
  --output string       output format: json, table, or text (default table)
  --query string        JMESPath expression to filter the result before printing
  -h, --help            show this help
`

func runRolesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "roles list", "print roles as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s roles list [flags]\n"+rolesFlagsHelp, prog, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	roles, err := client.ListRoles(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list roles: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, roles, func() { printStoredRolesTable(stdout, roles) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runRolesCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "roles create", "print the created role as JSON to stdout and nothing else", stderr)
	var abilitiesFlag, visibility, description string
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list, e.g. \"read,deploy\" (required)")
	fs.StringVar(&visibility, "visibility", "all", "all, or granted (sees only environments granted to each user; read ability only)")
	fs.StringVar(&description, "description", "", "short description")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s roles create <name> --abilities LIST [--visibility all|granted] [--description TEXT] [flags]\n"+
			"\n  --abilities string    comma-separated abilities (valid: read, read:sensitive, write, write:sensitive, deploy, root)\n"+
			"  --visibility string   all (default) or granted\n  --description string  short description\n"+rolesFlagsHelp, prog, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "roles create", "role name")
	if !ok {
		return exitUsage
	}
	if abilitiesFlag == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--abilities is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateRole(context.Background(), apiclient.RoleRequest{Name: name, Description: description, Abilities: splitAndTrim(abilitiesFlag), Visibility: visibility})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create role %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "role %q (id %s) created\n", created.Name, created.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runRolesUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "roles update", "print the updated role as JSON to stdout and nothing else", stderr)
	var name, abilitiesFlag, visibility, description string
	fs.StringVar(&name, "name", "", "new role name")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list (replaces the current one)")
	fs.StringVar(&visibility, "visibility", "", "all or granted")
	fs.StringVar(&description, "description", "", "new description")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s roles update <role> [--name N] [--abilities LIST] [--visibility V] [--description T] [flags]\n"+
			"\nOnly the flags you pass change. Built-in roles cannot be edited.\n"+rolesFlagsHelp, prog, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ref, ok := requireOneArg(fs, stderr, prog, "roles update", "role id or name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := findRole(ctx, client, ref)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	req := apiclient.RoleRequest{Name: cur.Name, Description: cur.Description, Abilities: cur.Abilities, Visibility: cur.Visibility}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["name"] {
		req.Name = name
	}
	if set["description"] {
		req.Description = description
	}
	if set["abilities"] {
		req.Abilities = splitAndTrim(abilitiesFlag)
	}
	if set["visibility"] {
		req.Visibility = visibility
	}
	updated, err := client.UpdateRole(ctx, cur.ID, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update role %q: %w", ref, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "role %q updated\n", updated.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runRolesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "roles delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s roles delete <role> [flags]\n\nRefused for built-in roles and roles that still have users.\n"+rolesFlagsHelp, prog, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ref, ok := requireOneArg(fs, stderr, prog, "roles delete", "role id or name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := findRole(ctx, client, ref)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := client.DeleteRole(ctx, cur.ID); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete role %q: %w", ref, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "role %q deleted\n", cur.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
