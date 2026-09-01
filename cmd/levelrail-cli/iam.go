package main

import (
	"fmt"
	"io"
)

// runIAM dispatches "iam policies <verb> [flags]": IAM-style resource-scoped
// Allow/Deny policy documents (internal/api/iam.go/iam_handlers.go),
// additive on top of a user's or token's flat --abilities list ("tokens
// create"/"users set-abilities"). "policies" is the only resource under
// "iam" today; the two-level shape (iam <resource> <verb>) mirrors AWS
// CLI's own "aws iam <verb>-policy" tree so a second IAM resource can be
// added later without a dispatch reshape.
func runIAM(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, iamUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, iamUsage(prog))
		return exitOK
	case "policies":
		return runIAMPolicies(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown iam subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, iamUsage(prog))
		return exitUsage
	}
}

func iamUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies <verb> [flags]

Run "%[1]s iam policies -h" for the full set of policy subcommands.
`, prog)
}

// runIAMPolicies dispatches "iam policies <verb> [flags]" to one of
// create/list/get/update/delete/attach/detach/attachments.
func runIAMPolicies(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, iamPoliciesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, iamPoliciesUsage(prog))
		return exitOK
	case "create":
		return runIAMPoliciesCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runIAMPoliciesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runIAMPoliciesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runIAMPoliciesUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runIAMPoliciesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "attach":
		return runIAMPoliciesAttach(prog, args[1:], stdout, stderr, lookupEnv)
	case "detach":
		return runIAMPoliciesDetach(prog, args[1:], stdout, stderr, lookupEnv)
	case "attachments":
		return runIAMPoliciesAttachments(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown iam policies subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, iamPoliciesUsage(prog))
		return exitUsage
	}
}

func iamPoliciesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s iam policies create --name NAME --document DOC [flags]                            create a policy
  %[1]s iam policies list [flags]                                                                     list policies
  %[1]s iam policies get <id> [flags]                                                                show one policy
  %[1]s iam policies update <id> --name NAME --document DOC [flags]                    replace a policy's name/description/document
  %[1]s iam policies delete <id> [flags]                                                            delete a policy
  %[1]s iam policies attach <id> --principal-type TYPE --principal-id ID [flags]        attach a policy to a user or token
  %[1]s iam policies detach <id> --principal-type TYPE --principal-id ID [flags]        detach a policy from a user or token
  %[1]s iam policies attachments <id> [flags]                                                     list a policy's attached principals

DOC is a policy document JSON string, e.g.:
  {"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}
Pass it inline, or as file://path/to/policy.json to read it from a file.

--principal-type must be "user" or "token"; --principal-id is the matching
user or token's own id.

Run "%[1]s iam policies <subcommand> -h" for a subcommand's own flags.
`, prog)
}
