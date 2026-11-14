package main

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
)

const errorsPagePath = "docs/iac/errors.md"

type codeDoc struct{ meaning, fix string }

// codeDocs holds the prose for each frozen code in internal/iac/codes.go.
// A test fails when a code has no entry, so a new code cannot ship undocumented.
var codeDocs = map[string]codeDoc{
	iac.CodeItemFailed:               {"The item failed and no more specific code applies.", "Read the message on the plan line; it names the resource and the cause."},
	iac.CodePermissionDenied:         {"The token may not read or write this resource.", "Use a token with the needed ability, or remove the resource from the files. Other items are not affected."},
	iac.CodeUnavailable:              {"The control plane does not serve this kind of resource.", "Check that the control plane runs a version that knows the kind."},
	iac.CodeReadFailed:               {"The live state of the resource could not be read, so it cannot be compared.", "Retry. If it repeats, check the control plane logs."},
	iac.CodeProtectedResource:        {"The resource is protected from this change, for example a protected app that prune would delete.", "Unprotect it, or declare it in the files."},
	iac.CodeImmutableField:           {"A declared field cannot change in place, such as a database engine or version.", "Recreate the resource deliberately, or set the field back to the live value."},
	iac.CodeSyncedExternally:         {"The resource is owned by an outside source, such as a pipeline synced from a repository.", "Change it at its source."},
	iac.CodeRefProjectMissing:        {"A resource names a project that does not exist and is not declared in the files.", "Add a `Project` document or fix the name."},
	iac.CodeRefEnvironmentMissing:    {"A resource names an environment that does not exist in the project and is not declared in the files.", "Add an `Environment` document or fix the name."},
	iac.CodeRefAppMissing:            {"A resource names an app that does not exist and is not declared in the files.", "Add the `App` document or fix the name."},
	iac.CodeRefSecretMissing:         {"A `secretRef` marked `existing: true` points at a secret the app does not have.", "Create the secret first, or drop `existing: true` and pass the value with `--secret`."},
	iac.CodeSchemaUnknownField:       {"A document has a field that is not part of its kind's schema.", "Check the spelling against the [reference](/iac/reference), or remove the field."},
	iac.CodeSchemaVersionUnsupported: {"The document `version` is not one this control plane reads.", "Set `version: 1`."},
	iac.CodeOwnershipConflict:        {"The resource is already owned by a different stack or source.", "Apply it from the stack or source that owns it, or release it there first."},
	iac.CodePlanStale:                {"The live state changed after the plan was made, so the plan no longer matches.", "Run the plan again and apply the new one."},
	iac.CodeStackLocked:              {"The resource belongs to a stack with drift policy `lock`, so manual changes are refused (HTTP 409).", "Change the manifest and apply it, or ask an administrator. See [Drift](/stacks#drift)."},
	iac.CodeStackSourceSync:          {"The stack is synced from a source, so a direct apply is refused.", "Change the source, not the stack."},
	iac.CodePruneGuard:               {"Prune was refused because it would delete more of the stack than the prune guard allows without confirmation.", "Check the plan, then confirm with `allow_mass_prune` if the deletions are intended."},
	iac.CodePlanHashVersion:          {"The submitted plan hash has the old unkeyed shape, which this control plane no longer accepts.", "Run the plan again to get a current hash."},
	iac.CodeInvalidRequest:           {"The request itself is malformed, for example a missing or unreadable field.", "Fix the request and send it again."},
	iac.CodeInvalidDocument:          {"A document failed validation.", "Read the issue lines; each names the file, line and field."},
	iac.CodeApprovalPending:          {"A cloud plan would replace the saved engine plan of a revision awaiting approval.", "Approve or reject the named revision first, or plan with supersede=true to retire it."},
	iac.CodePlanSuperseded:           {"A later plan replaced the saved engine plan this approval named.", "Plan again and request a new approval."},
	iac.CodeInterrupted:              {"The control plane stopped while the plan job ran.", "Plan again."},
	iac.CodePlanJobNotFound:          {"No plan job with this id exists for the stack, or it was removed after the retention window.", "Check the job id, or plan again."},
	iac.CodePlanTimeout:              {"The plan job ran past cloud.engine_timeout_seconds.", "Plan again, or raise cloud.engine_timeout_seconds."},
	iac.CodeApplyDisabled:            {"Apply is switched off for this control plane or stack.", "Ask an administrator to enable it."},
	iac.CodeSyncRepoUnreachable:      {"The git repository of a synced stack could not be fetched, or its credential could not be used.", "Check the repo URL, branch and credential_ref, then run a sync refresh."},
	iac.CodeSyncPathInvalid:          {"The sync path leaves the checkout, points into .git, or holds a symlink.", "Use a relative path with no .. and no symlinks."},
	iac.CodeSyncLimitsExceeded:       {"The repository or the files under the sync path passed a size or file count limit.", "Trim the path, or raise stacks.sync_max_repo_mb, stacks.sync_max_files or stacks.sync_max_file_kb."},
	iac.CodeSyncPlanFailed:           {"The documents under the sync path did not validate or could not be planned.", "Run the CLI plan on the same commit to see each issue."},
}

// renderErrors builds the error code page from the frozen code list.
func renderErrors(codes []string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\ntitle: \"IaC error codes\"\ntag: \"Experimental\"\ndescription: Every stable error code an apply, plan or stack call can return, what it means and how to fix it. Generated from the code list.\n---\n\n")
	b.WriteString("<!-- Generated by scripts/geniacref from internal/iac/codes.go. Do not edit. -->\n\n")
	b.WriteString("Codes are frozen: a released code is never renamed or reused, only added to. Match on the `code` field, not the message text.\n\n")
	b.WriteString("| Code | Meaning | What to do |\n| --- | --- | --- |\n")
	for _, c := range codes {
		d, ok := codeDocs[c]
		if !ok {
			return nil, fmt.Errorf("code %q has no entry in codeDocs", c)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c, d.meaning, d.fix)
	}
	return []byte(b.String()), nil
}
