package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// timestampDisplayFormat is the human-readable timestamp layout every
// non-JSON iam policies output uses.
const timestampDisplayFormat = "2006-01-02 15:04:05"

// resolveDocumentFlag reads a policy document flag value: file://path
// reads and returns that file's contents (AWS CLI's own file:// prefix
// convention for --policy-document and similar flags), anything else is
// used verbatim as inline JSON.
func resolveDocumentFlag(raw string) (string, error) {
	path, ok := strings.CutPrefix(raw, "file://")
	if !ok {
		return raw, nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied CLI flag, the same pattern apps_create.go's own --file read uses
	if err != nil {
		return "", fmt.Errorf("read policy document %s: %w", raw, err)
	}
	return string(b), nil
}

func runIAMPoliciesCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "iam policies create", "print the new policy as JSON to stdout and nothing else", stderr)
	var name, description, document string
	fs.StringVar(&name, "name", "", "policy name (required)")
	fs.StringVar(&description, "description", "", "policy description")
	fs.StringVar(&document, "document", "", "policy document JSON, or file://path (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamPoliciesCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if document == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--document is required"))
	}
	resolvedDoc, err := resolveDocumentFlag(document)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreatePolicy(context.Background(), policyRequest{
		Name:        name,
		Description: description,
		Document:    json.RawMessage(resolvedDoc),
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create policy %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "policy %q (id %s) created\n", created.Name, created.ID)
	return exitOK
}

func iamPoliciesCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies create --name NAME --document DOC [flags]

Flags:
  --name string             policy name (required)
  --description string   policy description
  --document string      policy document JSON, or file://path (required)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                        print the new policy as JSON to stdout, nothing else
  -h, --help                  show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMPoliciesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "iam policies list", "print policies as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamPoliciesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	policies, err := client.ListPolicies(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list policies: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, policies); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printPoliciesTable(stdout, policies)
	return exitOK
}

func printPoliciesTable(out io.Writer, policies []policyResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION\tUPDATED")
	for _, p := range policies {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Description, p.UpdatedAt.Format(timestampDisplayFormat))
	}
	_ = tw.Flush()
}

func iamPoliciesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies list [flags]

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print policies as a JSON array to stdout, nothing else
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMPoliciesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "iam policies get", "print the policy as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamPoliciesGetUsage(prog)) }

	client, id, jsonOut, code, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP}, stderr, singleArgCmd{prog, "iam policies get", "policy id"}, lookupEnv)
	if !ok {
		return code
	}

	got, err := client.GetPolicy(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get policy %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, got); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "id:          %s\n", got.ID)
	_, _ = fmt.Fprintf(stdout, "name:        %s\n", got.Name)
	_, _ = fmt.Fprintf(stdout, "description: %s\n", got.Description)
	_, _ = fmt.Fprintf(stdout, "document:    %s\n", got.Document)
	_, _ = fmt.Fprintf(stdout, "updated_at:  %s\n", got.UpdatedAt.Format(timestampDisplayFormat))
	return exitOK
}

func iamPoliciesGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies get <id> [flags]

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the policy as JSON to stdout, nothing else
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMPoliciesUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "iam policies update", "print the updated policy as JSON to stdout and nothing else", stderr)
	var name, description, document string
	fs.StringVar(&name, "name", "", "policy name (required)")
	fs.StringVar(&description, "description", "", "policy description")
	fs.StringVar(&document, "document", "", "policy document JSON, or file://path (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamPoliciesUpdateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "iam policies update", "policy id")
	if !ok {
		return exitUsage
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if document == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--document is required"))
	}
	resolvedDoc, err := resolveDocumentFlag(document)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.UpdatePolicy(context.Background(), id, policyRequest{
		Name:        name,
		Description: description,
		Document:    json.RawMessage(resolvedDoc),
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update policy %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "policy %q (id %s) updated\n", updated.Name, updated.ID)
	return exitOK
}

func iamPoliciesUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies update <id> --name NAME --document DOC [flags]

Flags:
  --name string             new policy name (required)
  --description string   new policy description
  --document string      new policy document JSON, or file://path (required)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                        print the updated policy as JSON to stdout, nothing else
  -h, --help                  show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMPoliciesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "iam policies delete", "print {\"deleted\":true} to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamPoliciesDeleteUsage(prog)) }

	client, id, jsonOut, code, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP}, stderr, singleArgCmd{prog, "iam policies delete", "policy id"}, lookupEnv)
	if !ok {
		return code
	}

	if err := client.DeletePolicy(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete policy %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "policy %q deleted\n", id)
	return exitOK
}

func iamPoliciesDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies delete <id> [flags]

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print {"deleted":true} to stdout, nothing else
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMPoliciesAttach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runIAMPoliciesAttachOrDetach(prog, args, stdout, stderr, lookupEnv, true)
}

func runIAMPoliciesDetach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runIAMPoliciesAttachOrDetach(prog, args, stdout, stderr, lookupEnv, false)
}

func runIAMPoliciesAttachOrDetach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), attach bool) int {
	verb := "detach"
	usage := iamPoliciesDetachUsage
	if attach {
		verb, usage = "attach", iamPoliciesAttachUsage
	}
	cmdLabel := "iam policies " + verb

	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, cmdLabel, "print {\"ok\":true} to stdout and nothing else", stderr)
	var principalType, principalID string
	fs.StringVar(&principalType, "principal-type", "", `"user" or "token" (required)`)
	fs.StringVar(&principalID, "principal-id", "", "the user or token's own id (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, cmdLabel, "policy id")
	if !ok {
		return exitUsage
	}
	if err := validatePrincipalFlags(principalType, principalID); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := attachOrDetachPolicy(client, id, principalType, principalID, attach); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s policy %q: %w", verb, id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"ok": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	preposition := "from"
	if attach {
		preposition = "to"
	}
	_, _ = fmt.Fprintf(stdout, "policy %q %sed %s %s %q\n", id, verb, preposition, principalType, principalID)
	return exitOK
}

func validatePrincipalFlags(principalType, principalID string) error {
	if principalType != "user" && principalType != "token" {
		return newValidationError(`--principal-type must be "user" or "token"`)
	}
	if principalID == "" {
		return newValidationError("--principal-id is required")
	}
	return nil
}

func attachOrDetachPolicy(client *Client, id, principalType, principalID string, attach bool) error {
	ctx := context.Background()
	if attach {
		return client.AttachPolicy(ctx, id, attachPolicyRequest{PrincipalType: principalType, PrincipalID: principalID})
	}
	return client.DetachPolicy(ctx, id, principalType, principalID)
}

func iamPoliciesAttachUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies attach <id> --principal-type TYPE --principal-id ID [flags]

Flags:
  --principal-type string   "user" or "token" (required)
  --principal-id string        the user or token's own id (required)
  --token string                  API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string               named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                              print {"ok":true} to stdout, nothing else
  -h, --help                       show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func iamPoliciesDetachUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies detach <id> --principal-type TYPE --principal-id ID [flags]

Flags:
  --principal-type string   "user" or "token" (required)
  --principal-id string        the user or token's own id (required)
  --token string                  API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string               named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                              print {"ok":true} to stdout, nothing else
  -h, --help                       show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMPoliciesAttachments(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "iam policies attachments", "print attachments as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamPoliciesAttachmentsUsage(prog)) }

	client, id, jsonOut, code, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP}, stderr, singleArgCmd{prog, "iam policies attachments", "policy id"}, lookupEnv)
	if !ok {
		return code
	}

	attachments, err := client.ListPolicyAttachments(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list attachments for policy %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, attachments); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PRINCIPAL_TYPE\tPRINCIPAL_ID\tATTACHED")
	for _, a := range attachments {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", a.PrincipalType, a.PrincipalID, a.CreatedAt.Format(timestampDisplayFormat))
	}
	_ = tw.Flush()
	return exitOK
}

func iamPoliciesAttachmentsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies attachments <id> [flags]

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print attachments as a JSON array to stdout, nothing else
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
